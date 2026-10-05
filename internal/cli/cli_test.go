package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/action/httpaction"
	"github.com/noi/dwbt/examples/users/mockapi"
)

// mockAPI serves the users API of examples/users, plus a slow endpoint for
// timeout tests.
func mockAPI(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle("/api/", mockapi.New())
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func app(dir string, environ ...string) (*App, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return &App{
		Stdout:  &stdout,
		Stderr:  &stderr,
		Dir:     dir,
		Environ: environ,
		Presets: map[string]action.Action{"http": httpaction.New()},
	}, &stdout, &stderr
}

// writeFiles creates files under a temporary directory and returns it.
func writeFiles(t *testing.T, files map[string]string) string {
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestExample(t *testing.T) {
	srv := mockAPI(t)
	dir := filepath.Join("..", "..", "examples", "users")

	a, stdout, stderr := app(dir)
	if code := a.Main(context.Background(), []string{"validate"}); code != ExitOK {
		t.Fatalf("validate: exit %d\n%s", code, stderr)
	}
	if !strings.Contains(stdout.String(), "ok: 2 workflow(s), 2 action(s)") {
		t.Errorf("validate output:\n%s", stdout)
	}

	a, stdout, stderr = app(dir)
	code := a.Main(context.Background(), []string{"run", "--server", "api=" + srv.URL})
	if code != ExitOK {
		t.Fatalf("run: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"=== .dwbt/workflows/follow.yaml - ユーザーをフォローできる",
		"  ok    prepare (user/create)",
		"  ok    #2 (user/follow)",
		"  ok    #3 (http)",
		"2 workflow(s): 2 passed, 0 failed, 0 errored",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, stdout)
		}
	}

	// The ci profile reads the URL from an environment variable, and a single
	// workflow can be selected by path.
	a, stdout, stderr = app(dir, "API_URL="+srv.URL)
	code = a.Main(context.Background(), []string{"run", ".dwbt/workflows/follow.yaml", "--env", "ci"})
	if code != ExitOK || !strings.Contains(stdout.String(), "1 workflow(s): 1 passed") {
		t.Fatalf("run --env ci: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

func TestExitCodes(t *testing.T) {
	srv := mockAPI(t)
	config := "default: local\nhttp:\n  timeout: 100ms\nenvironments:\n  local:\n    servers:\n      api: " + srv.URL + "\n"
	tests := []struct {
		name   string
		files  map[string]string
		args   []string
		code   int
		stdout []string
		stderr []string
	}{
		{
			name: "expectation failure",
			files: map[string]string{
				".dwbt/workflows/a.yaml": `
steps:
  - use: http
    params: { server: api, method: GET, path: /api/users/1/followers }
    expects:
      - status: 200
  - use: http
    params: { server: api, method: GET, path: /api/users/1/followers }
`,
			},
			args: []string{"run"},
			code: ExitFailure,
			stdout: []string{
				"  FAIL  #1 (http)",
				".dwbt/workflows/a.yaml:6:17: expects[0].status: expected 200, got 404",
				"res.req: GET " + srv.URL + "/api/users/1/followers",
				"  skip  #2 (http)",
				"0 passed, 1 failed, 0 errored",
			},
		},
		{
			name: "timeout",
			files: map[string]string{
				".dwbt/workflows/a.yaml": "steps:\n  - use: http\n    params: { server: api, method: GET, path: /slow }\n",
			},
			args:   []string{"run"},
			code:   ExitError,
			stdout: []string{"  ERROR #1 (http)", "timed out after 100ms", "0 passed, 0 failed, 1 errored"},
		},
		{
			name: "connection error",
			files: map[string]string{
				".dwbt/workflows/a.yaml": "steps:\n  - use: http\n    params: { server: api, method: GET, path: / }\n",
			},
			args:   []string{"run", "--server", "api=http://127.0.0.1:1"},
			code:   ExitError,
			stdout: []string{"  ERROR #1 (http)", "connection refused"},
		},
		{
			name: "definition errors",
			files: map[string]string{
				".dwbt/workflows/a.yaml":   "steps:\n  - use: nope\n",
				".dwbt/workflows/b.yaml":   "steps:\n  - use: http\n    params: { server: api }\n",
				".dwbt/workflows/c.yml":    "steps: []\n",
				".dwbt/actions/bad.yaml":   "params: {x: int}\nsteps: []\n",
				".dwbt/workflows/d.yaml":   "steps: [\n",
				".dwbt/workflows/e.yaml":   "steps:\n  - use: http\n    params: { server: api, method: GET, path: <<outputs.x>> }\n",
				".dwbt/workflows/sub/f.ok": "ignored",
			},
			args: []string{"validate"},
			code: ExitError,
			stderr: []string{
				"warning: .dwbt/workflows/c.yml: ignored; use the .yaml extension",
				`error: .dwbt/actions/bad.yaml:1:13: unknown type "int"`,
				"error: .dwbt/workflows/d.yaml",
			},
		},
		{
			name: "check errors",
			files: map[string]string{
				".dwbt/workflows/a.yaml": "steps:\n  - use: nope\n",
				".dwbt/workflows/b.yaml": "steps:\n  - use: http\n    params: { server: api, method: GET, path: <<outputs.x>> }\n",
			},
			args: []string{"run"},
			code: ExitError,
			stderr: []string{
				`error: .dwbt/workflows/a.yaml:2:10: unknown action "nope"`,
				`error: .dwbt/workflows/b.yaml:3:47: outputs.x: outputs of other steps can only be received via inputs`,
			},
		},
		{
			name:   "unknown environment",
			files:  map[string]string{".dwbt/workflows/a.yaml": "steps: []\n"},
			args:   []string{"run", "--env", "prod"},
			code:   ExitError,
			stderr: []string{`environment "prod" is not defined (available: local)`},
		},
		{
			name:   "invalid parallel",
			files:  map[string]string{".dwbt/workflows/a.yaml": "steps: []\n"},
			args:   []string{"run", "--parallel", "0"},
			code:   ExitError,
			stderr: []string{"--parallel must be at least 1, got 0"},
		},
		{
			name:   "unknown command",
			files:  map[string]string{".dwbt/workflows/a.yaml": "steps: []\n"},
			args:   []string{"deploy"},
			code:   ExitError,
			stderr: []string{`unknown command "deploy"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.files[".dwbt/config.yaml"] = config
			dir := writeFiles(t, tt.files)
			a, stdout, stderr := app(dir)
			code := a.Main(context.Background(), tt.args)
			if code != tt.code {
				t.Errorf("exit %d, want %d", code, tt.code)
			}
			for _, w := range tt.stdout {
				if !strings.Contains(stdout.String(), w) {
					t.Errorf("stdout does not contain %q", w)
				}
			}
			for _, w := range tt.stderr {
				if !strings.Contains(stderr.String(), w) {
					t.Errorf("stderr does not contain %q", w)
				}
			}
			if t.Failed() {
				t.Logf("stdout:\n%s\nstderr:\n%s", stdout, stderr)
			}
		})
	}
}

// barrier returns a server whose requests wait until n of them arrive. A
// request that waits for too long fails with 504.
func barrier(t *testing.T, n int) *httptest.Server {
	var mu sync.Mutex
	arrived := 0
	all := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		arrived++
		if arrived == n {
			close(all)
		}
		mu.Unlock()
		select {
		case <-all:
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusGatewayTimeout)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestParallel(t *testing.T) {
	// The workflows pass only when all of them run at the same time.
	const n = 3
	srv := barrier(t, n)
	files := map[string]string{}
	for _, name := range []string{"a", "b", "c"} {
		files[".dwbt/workflows/"+name+".yaml"] = `
steps:
  - use: http
    params: { server: api, method: GET, path: / }
    expects:
      - status: 200
`
	}
	dir := writeFiles(t, files)

	a, stdout, stderr := app(dir)
	code := a.Main(context.Background(), []string{"run", "--server", "api=" + srv.URL, "--parallel", "3"})
	if code != ExitOK {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	// The report of each workflow is not interleaved with the others.
	blocks := strings.Split(strings.TrimPrefix(stdout.String(), "=== "), "\n=== ")
	if len(blocks) != n {
		t.Fatalf("got %d reports, want %d:\n%s", len(blocks), n, stdout)
	}
	for _, b := range blocks {
		lines := strings.Split(b, "\n")
		if len(lines) < 3 || !strings.HasPrefix(lines[1], "  ok    #1 (http)") || !strings.HasPrefix(lines[2], "--- ok "+lines[0]+" ") {
			t.Errorf("unexpected report:\n=== %s", b)
		}
	}
	if !strings.Contains(stdout.String(), "3 workflow(s): 3 passed, 0 failed, 0 errored") {
		t.Errorf("stdout:\n%s", stdout)
	}
}

func TestFindRootFromSubdirectory(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		".dwbt/workflows/a.yaml": "steps:\n  - use: http\n    params: { server: api, method: GET, path: / }\n",
		"sub/dir/.keep":          "",
	})
	a, stdout, stderr := app(filepath.Join(dir, "sub", "dir"))
	if code := a.Main(context.Background(), []string{"validate"}); code != ExitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if !strings.Contains(stdout.String(), "ok: 1 workflow(s)") {
		t.Errorf("stdout:\n%s", stdout)
	}
}
