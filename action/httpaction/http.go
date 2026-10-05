// Package httpaction implements the built-in http action.
package httpaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/internal/value"
)

// Action is the http action.
//
// The URL of a server is either an http(s) URL or the path of a Unix domain
// socket, given as unix:///path/to/app.sock or unix:relative/app.sock.
type Action struct {
	// Client is used to send requests. If nil, a client with the runtime's
	// HTTP timeout is used. For a server on a Unix domain socket, its
	// Transport must be nil or an *http.Transport, which is cloned to dial
	// the socket.
	Client *http.Client

	mu sync.Mutex
	// unix holds the transports for Unix domain sockets by socket path.
	unix map[string]*http.Transport
}

// New returns the http action.
func New() *Action { return &Action{} }

func (*Action) Kind() string { return "http" }

func (*Action) Params() action.ParamSpec {
	return action.ParamSpec{
		"server":  {Type: action.String, Required: true},
		"method":  {Type: action.String, Required: true},
		"path":    {Type: action.String, Required: true},
		"query":   {Type: action.Object},
		"headers": {Type: action.Object},
		"json":    {Type: action.Any},
		"body":    {Type: action.String},
	}
}

func (*Action) Outputs() []string { return []string{"res"} }

func (a *Action) Run(ctx context.Context, rt action.Runtime, params map[string]any) (map[string]any, error) {
	req, sent, socket, err := buildRequest(ctx, rt, params)
	if err != nil {
		return nil, err
	}
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: rt.HTTPTimeout()}
	}
	target := req.URL.String()
	if socket != "" {
		tr, err := a.unixTransport(client, socket)
		if err != nil {
			return nil, err
		}
		c := *client
		c.Transport = tr
		client = &c
		target += " via unix:" + socket
	}
	resp, err := client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) && uerr.Timeout() {
			return nil, fmt.Errorf("%s %s: timed out after %s", req.Method, target, rt.HTTPTimeout())
		}
		return nil, fmt.Errorf("%s %s: %w", req.Method, target, unwrapURLError(err))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s: reading response: %w", req.Method, target, err)
	}
	var body any = string(data)
	if v, err := value.FromJSON(data); err == nil {
		body = v
	}
	res := map[string]any{
		"status":  resp.StatusCode,
		"headers": headerMap(resp.Header),
		"body":    body,
		"req":     sent,
	}
	return map[string]any{"res": res}, nil
}

// unixTransport returns the transport of client that dials socket. It is
// reused across requests so that connections are kept alive.
func (a *Action) unixTransport(client *http.Client, socket string) (*http.Transport, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if tr, ok := a.unix[socket]; ok {
		return tr, nil
	}
	var tr *http.Transport
	switch t := client.Transport.(type) {
	case nil:
		tr = http.DefaultTransport.(*http.Transport).Clone()
	case *http.Transport:
		tr = t.Clone()
	default:
		return nil, fmt.Errorf("unix:%s: Client.Transport must be an *http.Transport to dial a Unix domain socket, got %T", socket, t)
	}
	tr.Proxy = nil
	tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", socket)
	}
	if a.unix == nil {
		a.unix = map[string]*http.Transport{}
	}
	a.unix[socket] = tr
	return tr, nil
}

// parseServer splits the URL of a server into the base URL of requests and
// the path of the Unix domain socket to dial, which is empty for TCP.
func parseServer(id, raw string) (base, socket string, err error) {
	scheme, rest, ok := strings.Cut(raw, ":")
	if !ok || !strings.EqualFold(scheme, "unix") {
		return raw, "", nil
	}
	socket = rest
	if strings.HasPrefix(rest, "//") {
		host, path, _ := strings.Cut(rest[2:], "/")
		if host != "" {
			return "", "", fmt.Errorf("server %q: invalid URL %q: use unix:///absolute/path or unix:relative/path", id, raw)
		}
		socket = "/" + path
	}
	if socket == "" || socket == "/" {
		return "", "", fmt.Errorf("server %q: URL %q has no socket path", id, raw)
	}
	return "http://localhost", socket, nil
}

func buildRequest(ctx context.Context, rt action.Runtime, params map[string]any) (*http.Request, map[string]any, string, error) {
	jsonBody, hasJSON := params["json"]
	rawBody, hasBody := params["body"]
	if hasJSON && hasBody {
		return nil, nil, "", errors.New("json and body cannot be specified together")
	}

	id := params["server"].(string)
	raw, err := rt.Server(id)
	if err != nil {
		return nil, nil, "", err
	}
	base, socket, err := parseServer(id, raw)
	if err != nil {
		return nil, nil, "", err
	}
	path := params["path"].(string)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil {
		return nil, nil, "", fmt.Errorf("invalid URL: %w", err)
	}
	if q, ok := params["query"].(map[string]any); ok {
		values := u.Query()
		for _, k := range slices.Sorted(maps.Keys(q)) {
			for _, v := range listOf(q[k]) {
				values.Add(k, value.String(v))
			}
		}
		u.RawQuery = values.Encode()
	}

	var body io.Reader
	var sentBody any
	switch {
	case hasJSON:
		b, err := json.Marshal(jsonBody)
		if err != nil {
			return nil, nil, "", fmt.Errorf("encoding json: %w", err)
		}
		body = bytes.NewReader(b)
		sentBody = jsonBody
	case hasBody:
		body = strings.NewReader(rawBody.(string))
		sentBody = rawBody
	}

	method := strings.ToUpper(params["method"].(string))
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, nil, "", err
	}
	if h, ok := params["headers"].(map[string]any); ok {
		for _, k := range slices.Sorted(maps.Keys(h)) {
			for _, v := range listOf(h[k]) {
				req.Header.Add(k, value.String(v))
			}
		}
	}
	if hasJSON && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	sent := map[string]any{
		"method":  method,
		"url":     u.String(),
		"headers": headerMap(req.Header),
		"body":    sentBody,
	}
	return req, sent, socket, nil
}

func listOf(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return []any{v}
}

func headerMap(h http.Header) map[string]any {
	out := make(map[string]any, len(h))
	for k, vs := range h {
		out[strings.ToLower(k)] = strings.Join(vs, ", ")
	}
	return out
}

func unwrapURLError(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}
