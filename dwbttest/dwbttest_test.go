package dwbttest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/dwbttest"
)

// record is an action implemented in Go that records the values it is
// called with.
type record struct {
	mu   *sync.Mutex
	vals *[]string
}

func newRecord() record { return record{mu: &sync.Mutex{}, vals: &[]string{}} }

func (record) Params() action.ParamSpec {
	return action.ParamSpec{"value": {Type: action.String, Required: true}}
}

func (record) Outputs() []string { return []string{"value"} }

func (r record) Run(_ context.Context, _ action.Runtime, params map[string]any) (map[string]any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	*r.vals = append(*r.vals, params["value"].(string))
	return map[string]any{"value": params["value"]}, nil
}

func (r record) values() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(*r.vals)
}

// writeDwbt creates a .dwbt directory with files under a temporary directory
// and returns its path.
func writeDwbt(t *testing.T, files map[string]string) string {
	dir := filepath.Join(t.TempDir(), ".dwbt")
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

const recordWorkflow = `
steps:
  - use: test/record
    params:
      value: <<env.DWBTTEST_NAME>>-%s
    expects:
      - asserts:
          - outputs.current.value endsWith "-%s"
`

func TestRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"env": "` + r.URL.Query().Get("env") + `"}`))
	}))
	defer srv.Close()
	t.Setenv("DWBTTEST_NAME", "x")

	dir := writeDwbt(t, map[string]string{
		"config.yaml": `
environments:
  local:
    servers:
      api: http://127.0.0.1:1
`,
		"workflows/a.yaml":     strings.ReplaceAll(recordWorkflow, "%s", "a"),
		"workflows/sub/b.yaml": strings.ReplaceAll(recordWorkflow, "%s", "b"),
		"workflows/http.yaml": `
description: uses the overridden server
steps:
  - use: http
    params:
      server: api
      method: GET
      path: /
      query: { env: "<<env.DWBTTEST_NAME>>" }
    expects:
      - status: 200
        asserts:
          - outputs.current.res.body.env == "x"
`,
	})

	rec := newRecord()
	base := dwbttest.New(dir).Action("test/record", rec)

	// All the workflows run in the order of their paths.
	base.Server("api", srv.URL).Run(t)
	if got, want := rec.values(), []string{"x-a", "x-b"}; !slices.Equal(got, want) {
		t.Errorf("recorded %q, want %q", got, want)
	}

	// Workflows are selected relative to the workflows directory and the
	// receiver is left unchanged.
	selected := base.Workflows("sub/b.yaml")
	selected.Workflows("a.yaml").Run(t)
	selected.Run(t)
	if got, want := rec.values()[2:], []string{"x-b", "x-a", "x-b"}; !slices.Equal(got, want) {
		t.Errorf("recorded %q, want %q", got, want)
	}
}

func TestEnv(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	dir := writeDwbt(t, map[string]string{
		"config.yaml": `
default: local
environments:
  local:
    servers:
      api: http://127.0.0.1:1
  ci:
    servers:
      api: ` + srv.URL + `
`,
		"workflows/a.yaml": `
steps:
  - use: http
    params: { server: api, method: GET, path: / }
    expects:
      - status: 200
`,
	})
	dwbttest.New(dir).Env("ci").Run(t)
}

// TestReport runs failing suites in a subprocess and checks how they are
// reported.
func TestReport(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a subprocess")
	}
	tests := []struct {
		name string
		fail bool // whether the subprocess fails
		want []string
	}{
		{
			name: "failure",
			fail: true,
			want: []string{
				"--- PASS: TestHelper/failure/a.yaml",
				"--- FAIL: TestHelper/failure/b.yaml",
				"FAIL .dwbt/workflows/b.yaml - fails",
				"  ok    #1 (test/record)",
				"  FAIL  #2 (test/record)",
				`.dwbt/workflows/b.yaml:10:13: expects[0].asserts[0] failed: outputs.current.value == "b"`,
				"  skip  #3 (test/record)",
				"--- FAIL: TestHelper/failure/c.yaml",
				`  ERROR #1 (http)`,
				`server "api" is not defined; set it in config.yaml or override it with Server("api", url)`,
			},
		},
		{
			name: "definition errors",
			fail: true,
			want: []string{
				`error: .dwbt/workflows/a.yaml:3:10: unknown action "nope"`,
				"warning: .dwbt/workflows/b.yml: ignored; use the .yaml extension",
			},
		},
		{
			name: "invalid action",
			fail: true,
			want: []string{
				`error: action "http" is already registered`,
				`error: invalid action name "Bad Name"`,
			},
		},
		{
			name: "missing directory",
			fail: true,
			want: []string{"nope/.dwbt is not a directory"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestHelper$/^"+strings.ReplaceAll(tt.name, " ", "_")+"$", "-test.v")
			cmd.Env = append(os.Environ(), "DWBTTEST_HELPER=1")
			out, err := cmd.CombinedOutput()
			if (err != nil) != tt.fail {
				t.Errorf("err = %v", err)
			}
			for _, w := range tt.want {
				if !strings.Contains(string(out), w) {
					t.Errorf("output does not contain %q", w)
				}
			}
			if t.Failed() {
				t.Logf("output:\n%s", out)
			}
		})
	}
}

// TestHelper runs the suites of TestReport in a subprocess.
func TestHelper(t *testing.T) {
	if os.Getenv("DWBTTEST_HELPER") == "" {
		t.Skip("run by TestReport")
	}
	t.Run("failure", func(t *testing.T) {
		dir := writeDwbt(t, map[string]string{
			"workflows/a.yaml": "steps:\n  - use: test/record\n    params: { value: a }\n",
			"workflows/b.yaml": `
description: fails
steps:
  - use: test/record
    params: { value: a }
  - use: test/record
    params: { value: a }
    expects:
      - asserts:
          - outputs.current.value == "b"
  - use: test/record
    params: { value: c }
`,
			"workflows/c.yaml": "steps:\n  - use: http\n    params: { server: api, method: GET, path: / }\n",
		})
		t.Chdir(filepath.Dir(dir))
		dwbttest.New(".dwbt").Action("test/record", newRecord()).Run(t)
	})
	t.Run("definition_errors", func(t *testing.T) {
		dir := writeDwbt(t, map[string]string{
			"workflows/a.yaml": "\nsteps:\n  - use: nope\n",
			"workflows/b.yml":  "steps: []\n",
		})
		t.Chdir(filepath.Dir(dir))
		dwbttest.New(".dwbt").Run(t)
		t.Error("not reached")
	})
	t.Run("invalid_action", func(t *testing.T) {
		dir := writeDwbt(t, map[string]string{"workflows/a.yaml": "steps: []\n"})
		dwbttest.New(dir).Action("http", newRecord()).Action("Bad Name", newRecord()).Run(t)
		t.Error("not reached")
	})
	t.Run("missing_directory", func(t *testing.T) {
		dwbttest.New("nope/.dwbt").Run(t)
	})
}
