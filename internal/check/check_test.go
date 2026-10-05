package check

import (
	"strings"
	"testing"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/action/httpaction"
	"github.com/noi/dwbt/internal/def"
	"github.com/noi/dwbt/internal/yamlx"
)

var presets = map[string]action.Action{"http": httpaction.New()}

func project(t *testing.T, actions map[string]string) *def.Project {
	t.Helper()
	p := &def.Project{Actions: map[string]*def.ActionDef{}}
	for name, src := range actions {
		n, err := yamlx.Parse([]byte(src), name+".yaml")
		if err != nil {
			t.Fatal(err)
		}
		d, err := def.ParseAction(n, name)
		if err != nil {
			t.Fatal(err)
		}
		p.Actions[name] = d
	}
	return p
}

func workflow(t *testing.T, src string) *def.Workflow {
	t.Helper()
	n, err := yamlx.Parse([]byte(src), "wf.yaml")
	if err != nil {
		t.Fatal(err)
	}
	wf, err := def.ParseWorkflow(n, "wf.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return wf
}

const userCreate = `
params:
  name: string
  age: number
steps:
  - id: create
    use: http
    params:
      server: api
      method: POST
      path: /api/users
      json:
        name: <<params.name>>
        age: <<params.age>>
    expects:
      - status: 200
    outputs:
      user: <<outputs.current.res.body>>
outputs:
  user: <<outputs.steps.create.user>>
`

func TestCheckValid(t *testing.T) {
	p := project(t, map[string]string{"user/create": userCreate})
	wf := workflow(t, `
steps:
  - id: prepare
    use: user/create
    foreach: [{ name: a, age: 18 }, { name: b, age: 19 }]
    params:
      name: <<item.name>>
      age: <<item.age + index>>
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
      path: /api/users/<<inputs.users[1].id>>/followers
      headers:
        X-Env: <<env.HOME>>
    expects:
      - type: http
        status: 200
        asserts:
          - any(outputs.current.res.body, .id == inputs.users[0].id)
`)
	if err := Check(p, presets, []*def.Workflow{wf}); err != nil {
		t.Fatalf("unexpected errors:\n%s", joinErrs(err))
	}
}

func TestCheckErrors(t *testing.T) {
	tests := []struct {
		name    string
		actions map[string]string
		wf      string
		want    []string
	}{
		{
			name:    "scope",
			actions: map[string]string{"user/create": userCreate},
			wf: `
steps:
  - id: prepare
    use: user/create
    params:
      name: <<item.name>>
      age: <<params.age>>
    outputs:
      users: <<outputs.current.user>>
  - id: next
    use: http
    inputs:
      a: <<outputs.prepare.nope>>
      b: <<outputs.later.x>>
      c: <<inputs.a>>
      d: <<outputs.current.user>>
    params:
      server: api
      method: GET
      path: /<<outputs.prepare.users>>/<<inputs.zzz>>/<<foo>>
    expects:
      - asserts:
          - outputs.current.nope == 1
          - outputs == nil
    outputs:
      x: 1
  - id: later
    use: http
    params: { server: api, method: GET, path: / }
    outputs:
      x: 1
`,
			want: []string{
				`wf.yaml:6:13: item.name: item can only be used in params, expects and outputs of a step with foreach`,
				`wf.yaml:7:12: params.age: params can only be referenced inside an action`,
				`wf.yaml:13:10: outputs.prepare.nope: step "prepare" does not publish output "nope"`,
				`wf.yaml:14:10: outputs.later.x: no step with id "later" and outputs precedes this step`,
				`wf.yaml:15:10: inputs.a: inputs cannot be referenced in inputs`,
				`wf.yaml:16:10: outputs.current.user: outputs.current is not available in inputs`,
				`wf.yaml:20:13: outputs.prepare.users: outputs of other steps can only be received via inputs`,
				`wf.yaml:20:13: inputs.zzz: undeclared input "zzz"`,
				`wf.yaml:20:13: foo: unknown variable "foo"`,
				`wf.yaml:23:13: outputs.current.nope: the action does not publish output "nope"`,
				`wf.yaml:24:13: outputs: outputs must be followed by current, steps or a step id`,
			},
		},
		{
			name:    "signature and expects",
			actions: map[string]string{"user/create": userCreate},
			wf: `
steps:
  - use: user/create
    params:
      name: 1
      extra: x
    expects:
      - type: http
      - status: 200
  - use: http
    params: { server: api, method: GET }
    expects:
      - type: grpc
    outputs:
      x: 1
  - use: nope
  - id: current
    use: http
    params: { server: api, method: GET, path: / }
  - id: dup
    use: http
    params: { server: api, method: GET, path: / }
  - id: dup
    use: http
    params: { server: api, method: GET, path: / }
  - id: steps
    use: http
    params: { server: api, method: GET, path: <<outputs.steps.dup>> }
`,
			want: []string{
				`wf.yaml:5:13: parameter "name" must be string`,
				`wf.yaml:6:7: action "user/create" has no parameter "extra"`,
				`wf.yaml:3:5: missing parameters for action "user/create": age`,
				`wf.yaml:8:15: expectation type "http" is not available for action "user/create"`,
				`wf.yaml:9:17: status is only available for http expectations`,
				`wf.yaml:10:5: missing parameters for action "http": path`,
				`wf.yaml:13:15: unknown expectation type "grpc"`,
				`wf.yaml:10:5: a step with outputs must have an id`,
				`wf.yaml:16:10: unknown action "nope"`,
				`wf.yaml:17:9: step id "current" is reserved`,
				`wf.yaml:23:9: duplicate step id "dup"`,
				`wf.yaml:26:9: step id "steps" is reserved`,
				`wf.yaml:28:47: outputs.steps.dup: outputs.steps is only available in action outputs`,
			},
		},
		{
			name: "action definitions",
			actions: map[string]string{
				"http": `steps: [{use: http, params: {server: a, method: GET, path: /}}]`,
				"a":    `steps: [{use: b}]`,
				"b":    `steps: [{use: a}]`,
				"c": `
params:
  x: string
steps:
  - id: s
    use: http
    params: { server: a, method: GET, path: /<<params.y>> }
    outputs:
      v: 1
outputs:
  o: <<outputs.steps.s.w>>
  p: <<outputs.steps.t>>
  q: <<outputs.s.v>>
  r: <<outputs.current.s.v>>
`,
			},
			wf: `
actions:
  a:
    steps: [{use: http, params: {server: a, method: GET, path: /}}]
  local:
    steps: [{use: local}]
steps:
  - use: local
`,
			want: []string{
				`http.yaml:1:1: action "http" conflicts with a built-in action`,
				`actions call each other in a cycle: a -> b -> a`,
				`c.yaml:7:45: params.y: undeclared parameter "y"`,
				`c.yaml:11:6: outputs.steps.s.w: step "s" does not publish output "w"`,
				`c.yaml:12:6: outputs.steps.t: the action has no step with id "t" and outputs`,
				`c.yaml:13:6: outputs.s.v: outputs of other steps can only be received via inputs`,
				`c.yaml:14:6: outputs.current.s.v: outputs.current is not available in action outputs; use outputs.steps.<id> for the outputs of the action's steps`,
				`wf.yaml:3:3: action "a" is already defined in the actions directory`,
				`wf.yaml:5:3: actions call each other in a cycle: local -> local`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Check(project(t, tt.actions), presets, []*def.Workflow{workflow(t, tt.wf)})
			if err == nil {
				t.Fatal("no errors")
			}
			got := joinErrs(err)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q", w)
				}
			}
			if n := len(err.(def.Errors)); n != len(tt.want) {
				t.Errorf("got %d errors, want %d", n, len(tt.want))
			}
			if t.Failed() {
				t.Logf("errors:\n%s", got)
			}
		})
	}
}

func joinErrs(err error) string {
	var lines []string
	for _, e := range err.(def.Errors) {
		lines = append(lines, e.Error())
	}
	return strings.Join(lines, "\n")
}
