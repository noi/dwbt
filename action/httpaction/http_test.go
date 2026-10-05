package httpaction

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type runtime struct {
	servers map[string]string
	timeout time.Duration
}

func (r runtime) Server(id string) (string, error) {
	u, ok := r.servers[id]
	if !ok {
		return "", fmt.Errorf("unknown server %q", id)
	}
	return u, nil
}

func (r runtime) HTTPTimeout() time.Duration { return r.timeout }

func TestRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Add("X-Multi", "a")
		w.Header().Add("X-Multi", "b")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{
			"method": r.Method,
			"path":   r.URL.Path,
			"query":  r.URL.RawQuery,
			"ctype":  r.Header.Get("Content-Type"),
			"accept": r.Header.Values("Accept"),
			"body":   string(b),
			"id":     12345678901,
			"ratio":  0.5,
		})
	}))
	defer srv.Close()

	rt := runtime{servers: map[string]string{"api": srv.URL + "/"}, timeout: 5 * time.Second}
	out, err := New().Run(context.Background(), rt, map[string]any{
		"server":  "api",
		"method":  "post",
		"path":    "/api/users",
		"query":   map[string]any{"q": "a b", "tag": []any{"x", "y"}, "n": 2},
		"headers": map[string]any{"Accept": []any{"application/json", "text/plain"}},
		"json":    map[string]any{"name": "alice", "age": 18},
	})
	if err != nil {
		t.Fatal(err)
	}
	res := out["res"].(map[string]any)
	if res["status"] != 201 {
		t.Errorf("status = %v", res["status"])
	}
	if h := res["headers"].(map[string]any)["x-multi"]; h != "a, b" {
		t.Errorf("x-multi = %v", h)
	}
	body := res["body"].(map[string]any)
	want := map[string]any{
		"method": "POST",
		"path":   "/api/users",
		"query":  "n=2&q=a+b&tag=x&tag=y",
		"ctype":  "application/json",
		"accept": []any{"application/json", "text/plain"},
		"body":   `{"age":18,"name":"alice"}`,
		"id":     12345678901,
		"ratio":  0.5,
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("body = %#v\nwant %#v", body, want)
	}
	sent := res["req"].(map[string]any)
	if sent["url"] != srv.URL+"/api/users?n=2&q=a+b&tag=x&tag=y" || sent["method"] != "POST" {
		t.Errorf("req = %#v", sent)
	}
	if !reflect.DeepEqual(sent["body"], map[string]any{"name": "alice", "age": 18}) {
		t.Errorf("req.body = %#v", sent["body"])
	}
}

func TestRunTextBodyAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			time.Sleep(200 * time.Millisecond)
		}
		w.WriteHeader(404)
		io.WriteString(w, "not found")
	}))
	defer srv.Close()
	rt := runtime{servers: map[string]string{"api": srv.URL}, timeout: 50 * time.Millisecond}

	out, err := New().Run(context.Background(), rt, map[string]any{
		"server": "api", "method": "GET", "path": "missing",
	})
	if err != nil {
		t.Fatal(err)
	}
	res := out["res"].(map[string]any)
	if res["status"] != 404 || res["body"] != "not found" {
		t.Errorf("res = %#v", res)
	}

	_, err = New().Run(context.Background(), rt, map[string]any{
		"server": "api", "method": "GET", "path": "/slow",
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("slow: err = %v", err)
	}

	_, err = New().Run(context.Background(), rt, map[string]any{
		"server": "api", "method": "POST", "path": "/", "json": 1, "body": "x",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be specified together") {
		t.Errorf("json+body: err = %v", err)
	}

	_, err = New().Run(context.Background(), rt, map[string]any{
		"server": "nope", "method": "GET", "path": "/",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown server") {
		t.Errorf("unknown server: err = %v", err)
	}
}

func TestRunUnixSocket(t *testing.T) {
	// t.TempDir can exceed the length limit of socket paths on macOS.
	dir, err := os.MkdirTemp("", "dwbt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "api.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, r.Host+" "+r.URL.RequestURI())
	}))
	srv.Listener = ln
	srv.Start()
	defer srv.Close()

	t.Chdir(dir)
	a := New()
	for _, base := range []string{"unix://" + socket, "unix:" + socket, "unix:api.sock"} {
		rt := runtime{servers: map[string]string{"api": base}, timeout: 5 * time.Second}
		out, err := a.Run(context.Background(), rt, map[string]any{
			"server": "api", "method": "GET", "path": "/users", "query": map[string]any{"q": "a"},
		})
		if err != nil {
			t.Fatalf("%s: %v", base, err)
		}
		res := out["res"].(map[string]any)
		if res["status"] != 200 || res["body"] != "localhost /users?q=a" {
			t.Errorf("%s: res = %#v", base, res)
		}
	}

	for base, want := range map[string]string{
		"unix://host/x.sock":           "invalid URL",
		"unix:":                        "no socket path",
		"unix:///":                     "no socket path",
		"unix://" + dir + "/none.sock": "via unix:" + dir + "/none.sock",
	} {
		rt := runtime{servers: map[string]string{"api": base}, timeout: 5 * time.Second}
		_, err := a.Run(context.Background(), rt, map[string]any{
			"server": "api", "method": "GET", "path": "/",
		})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", base, err, want)
		}
	}
}
