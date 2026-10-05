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
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/internal/value"
)

// Action is the http action.
type Action struct {
	// Client is used to send requests. If nil, a client with the runtime's
	// HTTP timeout is used.
	Client *http.Client
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
	req, sent, err := buildRequest(ctx, rt, params)
	if err != nil {
		return nil, err
	}
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: rt.HTTPTimeout()}
	}
	resp, err := client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) && uerr.Timeout() {
			return nil, fmt.Errorf("%s %s: timed out after %s", req.Method, req.URL, rt.HTTPTimeout())
		}
		return nil, fmt.Errorf("%s %s: %w", req.Method, req.URL, unwrapURLError(err))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s: reading response: %w", req.Method, req.URL, err)
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

func buildRequest(ctx context.Context, rt action.Runtime, params map[string]any) (*http.Request, map[string]any, error) {
	jsonBody, hasJSON := params["json"]
	rawBody, hasBody := params["body"]
	if hasJSON && hasBody {
		return nil, nil, errors.New("json and body cannot be specified together")
	}

	base, err := rt.Server(params["server"].(string))
	if err != nil {
		return nil, nil, err
	}
	path := params["path"].(string)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid URL: %w", err)
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
			return nil, nil, fmt.Errorf("encoding json: %w", err)
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
		return nil, nil, err
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
	return req, sent, nil
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
