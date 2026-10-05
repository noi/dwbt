package def

import (
	"strings"
	"testing"
	"time"

	"github.com/noi/dwbt/internal/yamlx"
)

func parseWorkflow(t *testing.T, src string) (*Workflow, error) {
	t.Helper()
	n, err := yamlx.Parse([]byte(src), "wf.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return ParseWorkflow(n, "wf.yaml")
}

func TestParseWorkflow(t *testing.T) {
	wf, err := parseWorkflow(t, `
description: sample
actions:
  user-create:
    params:
      name: string
    steps:
      - id: create
        use: http
        params:
          server: api
          method: POST
          path: /api/users
          json:
            name: <<params.name>>
        outputs:
          user: <<outputs.current.res.body>>
    outputs:
      user: <<outputs.current.create.user>>
steps:
  - id: prepare
    use: user-create
    foreach: [{ name: a }, { name: b }]
    params:
      name: <<item.name>>
    expects:
      - asserts:
          - outputs.current.user.name == item.name
    outputs:
      users: <<outputs.current.user>>
  - use: http
    inputs:
      users: <<outputs.prepare.users>>
    params:
      server: api
      method: GET
      path: /api/users/<<inputs.users[0].id>>
    expects:
      - status: 200
`)
	if err != nil {
		t.Fatal(err)
	}
	if wf.Description != "sample" || len(wf.Steps) != 2 || len(wf.Actions) != 1 {
		t.Fatalf("unexpected workflow: %+v", wf)
	}
	a := wf.Actions["user-create"]
	if !a.Local || len(a.Params) != 1 || a.Params[0].Type != "string" || len(a.Outputs) != 1 {
		t.Errorf("unexpected action: %+v", a)
	}
	s := wf.Steps[1]
	if s.Label() != "#2 (http)" || *s.Expects[0].Status != 200 || len(s.Inputs) != 1 {
		t.Errorf("unexpected step: %+v", s)
	}
	if wf.Steps[0].Label() != "prepare (user-create)" || wf.Steps[0].Foreach == nil {
		t.Errorf("unexpected step: %+v", wf.Steps[0])
	}
}

func TestParseWorkflowErrors(t *testing.T) {
	_, err := parseWorkflow(t, `
steps:
  - id: bad-id
    use: http
    unknown: 1
    params: [1]
    expects:
      - status: "200"
        asserts:
          - <<x>>
          - "1 +"
    outputs:
      bad-name: 1
  - params: {}
`)
	errs, ok := err.(Errors)
	if !ok {
		t.Fatalf("err = %v", err)
	}
	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{
		`wf.yaml:3:9: invalid step id "bad-id"`,
		`wf.yaml:5:5: unknown key "unknown" in step`,
		`wf.yaml:6:13: params must be a mapping`,
		`wf.yaml:8:17: status must be an integer`,
		`wf.yaml:10:13: asserts are expressions`,
		`wf.yaml:11:13: invalid expression "1 +"`,
		`wf.yaml:13:7: invalid outputs name "bad-name"`,
		`wf.yaml:14:5: step must have use`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing error %q in:\n%s", want, all)
		}
	}
}

func TestParseConfig(t *testing.T) {
	n, err := yamlx.Parse([]byte(`
default: local
http:
  timeout: 5s
environments:
  local:
    servers:
      api: http://127.0.0.1:8080
  ci:
    servers:
      api: <<env.API_URL ?? "http://localhost">>
`), "config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseConfig(n)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Default != "local" || cfg.HTTPTimeout != 5*time.Second || len(cfg.Environments) != 2 {
		t.Errorf("unexpected config: %+v", cfg)
	}

	n, _ = yamlx.Parse([]byte(`
default: nope
environments:
  local:
    servers:
      api: <<inputs.x>>
`), "config.yaml")
	_, err = ParseConfig(n)
	if err == nil || !strings.Contains(err.Error(), "only env can be referenced") {
		t.Errorf("err = %v", err)
	}
	if errs := err.(Errors); len(errs) != 2 || !strings.Contains(errs[1].Error(), `default environment "nope" is not defined`) {
		t.Errorf("errs = %v", errs)
	}
}
