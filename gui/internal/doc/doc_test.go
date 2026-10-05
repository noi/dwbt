package doc

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const examples = "../../../examples/users/.dwbt"

func TestWorkflowRoundTrip(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(examples, "workflows", "*.yaml"))
	if len(files) == 0 {
		t.Fatal("no example workflows")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			wf, err := ParseWorkflow(data)
			if err != nil {
				t.Fatal(err)
			}
			out, err := EncodeWorkflow(wf)
			if err != nil {
				t.Fatal(err)
			}
			sameData(t, data, out)
		})
	}
}

func TestActionRoundTrip(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(examples, "actions", "*", "*.yaml"))
	if len(files) == 0 {
		t.Fatal("no example actions")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			a, err := ParseAction(data)
			if err != nil {
				t.Fatal(err)
			}
			out, err := EncodeAction(a)
			if err != nil {
				t.Fatal(err)
			}
			sameData(t, data, out)
		})
	}
}

func TestEncodeWorkflow(t *testing.T) {
	status := 201
	wf := &Workflow{
		Description: "作成できる",
		Actions: []*Action{{
			Name:    "local/ping",
			Params:  []*Param{{Name: "n", Type: "number"}},
			Steps:   []*Step{{Use: "http", Params: []*Binding{{Name: "server", Value: "api"}}}},
			Outputs: []*Binding{{Name: "v", Value: "<<params.n>>"}},
		}},
		Steps: []*Step{
			{
				ID:      "create",
				Use:     "http",
				Foreach: "[1, 2] # items",
				Params: []*Binding{
					{Name: "server", Value: "api"},
					{Name: "json", Value: "name: <<item>>\nage: 18"},
					{Name: "body", Value: ""},
					{Name: "query", Value: `"true"`},
				},
				Expects: []*Expect{{Status: &status, Asserts: []string{"true"}}},
				Outputs: []*Binding{{Name: "user", Value: "<<outputs.current.res.body>>"}},
			},
			{Use: "local/ping", Params: []*Binding{{Name: "n", Value: "1"}}},
		},
	}
	got, err := EncodeWorkflow(wf)
	if err != nil {
		t.Fatal(err)
	}
	want := `description: 作成できる
actions:
  local/ping:
    params:
      n: number
    steps:
      - use: http
        params:
          server: api
    outputs:
      v: <<params.n>>
steps:
  - id: create
    use: http
    foreach: [1, 2] # items
    params:
      server: api
      json:
        name: <<item>>
        age: 18
      body:
      query: "true"
    expects:
      - status: 201
        asserts:
          - "true"
    outputs:
      user: <<outputs.current.res.body>>

  - use: local/ping
    params:
      n: 1
`
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	back, err := ParseWorkflow(got)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodeWorkflow(back)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != want {
		t.Errorf("encoded again:\n%s", again)
	}
}

func TestParseDropsExpectationType(t *testing.T) {
	wf, err := ParseWorkflow([]byte("steps:\n  - use: http\n    expects:\n      - type: http\n        status: 200\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := EncodeWorkflow(wf)
	if err != nil {
		t.Fatal(err)
	}
	if want := "steps:\n  - use: http\n    expects:\n      - status: 200\n"; string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestEncodeInvalidValue(t *testing.T) {
	wf := &Workflow{Steps: []*Step{{Use: "http", Params: []*Binding{{Name: "json", Value: "a: [1"}}}}}
	_, err := EncodeWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "step 1: params.json:") {
		t.Errorf("got %v", err)
	}
}

func TestParseUnknownKey(t *testing.T) {
	_, err := ParseWorkflow([]byte("steps:\n  - use: http\n    when: x\n"))
	if err == nil || !strings.Contains(err.Error(), `line 3: unknown key "when" in step`) {
		t.Errorf("got %v", err)
	}
}

func TestParseEmpty(t *testing.T) {
	wf, err := ParseWorkflow(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(wf.Steps) != 0 || wf.Steps == nil {
		t.Errorf("got %#v", wf.Steps)
	}
}

func sameData(t *testing.T, a, b []byte) {
	t.Helper()
	var va, vb any
	if err := yaml.Unmarshal(a, &va); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(b, &vb); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(va, vb) {
		t.Errorf("data differs:\n%s\n---\n%s", a, b)
	}
}

func TestSetupTeardown(t *testing.T) {
	src := `setup:
  steps:
    - id: a
      use: user/create

    - id: b
      use: user/create
  outputs:
    a: <<outputs.steps.a.user>>
inputs:
  a: <<outputs.setup.a>>
steps:
  - use: http

  - use: http
teardown:
  - use: user/delete

  - use: user/delete
`
	wf, err := ParseWorkflow([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if wf.Setup == nil || len(wf.Setup.Steps) != 2 || len(wf.Setup.Outputs) != 1 || len(wf.Inputs) != 1 || len(wf.Teardown) != 2 {
		t.Fatalf("unexpected workflow: %+v", wf)
	}
	out, err := EncodeWorkflow(wf)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != src {
		t.Errorf("encoded:\n%s\nwant:\n%s", out, src)
	}

	if _, err := ParseWorkflow([]byte("setup:\n  nope: 1\n")); err == nil || !strings.Contains(err.Error(), `unknown key "nope" in setup`) {
		t.Errorf("err = %v", err)
	}
}
