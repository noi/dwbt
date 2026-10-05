package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/action/httpaction"
	"github.com/noi/dwbt/internal/def"
	"github.com/noi/dwbt/internal/yamlx"
)

type runtime struct{ url string }

func (r runtime) Server(id string) (string, error) {
	if id != "api" {
		return "", fmt.Errorf("unknown server %q", id)
	}
	return r.url, nil
}

func (runtime) HTTPTimeout() time.Duration { return 5 * time.Second }

// usersAPI is a minimal users API with followers.
func usersAPI(t *testing.T) *httptest.Server {
	var mu sync.Mutex
	users := map[int]map[string]any{}
	followers := map[int][]map[string]any{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		var u map[string]any
		json.NewDecoder(r.Body).Decode(&u)
		mu.Lock()
		u["id"] = len(users) + 1
		users[len(users)+1] = u
		mu.Unlock()
		json.NewEncoder(w).Encode(u)
	})
	mux.HandleFunc("POST /api/users/{id}/followers", func(w http.ResponseWriter, r *http.Request) {
		var id int
		fmt.Sscan(r.PathValue("id"), &id)
		var body struct {
			UserID int `json:"user_id"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		follower, ok := users[body.UserID]
		if !ok || users[id] == nil {
			w.WriteHeader(404)
			return
		}
		followers[id] = append(followers[id], follower)
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /api/users/{id}/followers", func(w http.ResponseWriter, r *http.Request) {
		var id int
		fmt.Sscan(r.PathValue("id"), &id)
		mu.Lock()
		defer mu.Unlock()
		list := followers[id]
		if list == nil {
			list = []map[string]any{}
		}
		json.NewEncoder(w).Encode(list)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

const actions = `
create:
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
    user: <<outputs.current.create.user>>
follow:
  params:
    follower: number
    followee: number
  steps:
    - use: http
      params:
        server: api
        method: POST
        path: /api/users/<<params.followee>>/followers
        json:
          user_id: <<params.follower>>
      expects:
        - status: 204
`

func run(t *testing.T, srv *httptest.Server, steps string) *Result {
	t.Helper()
	src := "actions:\n" + indent(actions) + "steps:\n" + indent(steps)
	n, err := yamlx.Parse([]byte(src), "wf.yaml")
	if err != nil {
		t.Fatal(err)
	}
	wf, err := def.ParseWorkflow(n, "wf.yaml")
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{
		Resolver: def.Resolver{Presets: map[string]action.Action{"http": httpaction.New()}},
		Runtime:  runtime{url: srv.URL},
		Env:      map[string]any{"PREFIX": "u"},
	}
	return r.Run(context.Background(), wf)
}

func indent(s string) string {
	lines := strings.Split(strings.Trim(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n") + "\n"
}

const followSteps = `
- id: prepare
  use: create
  foreach: [{ name: a, age: 18 }, { name: b, age: 19 }]
  params:
    name: <<env.PREFIX>>-<<item.name>>
    age: <<item.age>>
  expects:
    - asserts:
        - outputs.current.user.age == item.age
  outputs:
    users: <<outputs.current.user>>
    indexes: <<index>>
- use: follow
  inputs:
    users: <<outputs.prepare.users>>
  params:
    follower: <<inputs.users[0].id>>
    followee: <<inputs.users[1].id>>
- id: check
  use: http
  inputs:
    users: <<outputs.prepare.users>>
  params:
    server: api
    method: GET
    path: /api/users/<<inputs.users[1].id>>/followers
  expects:
    - status: 200
      asserts:
        - any(outputs.current.res.body, .id == inputs.users[0].id)
        - outputs.current.res.req.method == "GET"
  outputs:
    names: <<map(outputs.current.res.body, .name)>>
`

func TestRunPass(t *testing.T) {
	srv := usersAPI(t)
	res := run(t, srv, followSteps)
	if res.Status != Passed {
		for _, s := range res.Steps {
			t.Logf("%s %s %v", s.Status, s.Step.Label(), s.Err)
		}
		t.Fatalf("status = %s", res.Status)
	}
	if len(res.Steps) != 3 {
		t.Errorf("steps = %d", len(res.Steps))
	}
}

func TestRunFailure(t *testing.T) {
	srv := usersAPI(t)
	res := run(t, srv, `
- id: prepare
  use: create
  params: { name: a, age: 18 }
  outputs:
    user: <<outputs.current.user>>
- use: http
  inputs:
    user: <<outputs.prepare.user>>
  params:
    server: api
    method: GET
    path: /api/users/<<inputs.user.id>>/followers
  expects:
    - status: 200
      asserts:
        - len(outputs.current.res.body) == 1
- use: http
  params: { server: api, method: GET, path: / }
`)
	if res.Status != Failed {
		t.Fatalf("status = %s", res.Status)
	}
	got := []Status{res.Steps[0].Status, res.Steps[1].Status, res.Steps[2].Status}
	if got[0] != Passed || got[1] != Failed || got[2] != Skipped {
		t.Errorf("statuses = %v", got)
	}
	msg := res.Steps[1].Err.Error()
	for _, want := range []string{"wf.yaml:52:13: expects[0].asserts[0] failed: len(outputs.current.res.body) == 1", "outputs.current.res.body = []"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
}

func TestRunNestedFailureAndErrors(t *testing.T) {
	srv := usersAPI(t)
	tests := []struct {
		steps  string
		status Status
		want   string
	}{
		{
			steps:  `- use: follow` + "\n" + `  params: { follower: 1, followee: 99 }`,
			status: Failed,
			want:   "in action follow, step #1 (http): wf.yaml:35:21: expects[0].status: expected 204, got 404",
		},
		{
			steps:  `- use: create` + "\n" + `  params: { name: <<1>>, age: 1 }`,
			status: Errored,
			want:   `parameter "name" of action "create" must be string, got number`,
		},
		{
			steps:  `- use: create` + "\n" + `  foreach: <<"x">>` + "\n" + `  params: { name: a, age: 1 }`,
			status: Errored,
			want:   "foreach must be an array, got string",
		},
		{
			steps:  `- use: http` + "\n" + `  params: { server: api, method: GET, path: / }` + "\n" + `  expects: [{asserts: ["1"]}]`,
			status: Errored,
			want:   "assert must evaluate to bool, got number",
		},
		{
			steps:  `- use: http` + "\n" + `  params: { server: nope, method: GET, path: / }`,
			status: Errored,
			want:   `http: unknown server "nope"`,
		},
	}
	for _, tt := range tests {
		res := run(t, srv, tt.steps)
		if res.Status != tt.status {
			t.Errorf("%s: status = %s, want %s (%v)", tt.steps, res.Status, tt.status, res.Steps[0].Err)
			continue
		}
		if err := res.Steps[0].Err; err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want containing %q", tt.steps, err, tt.want)
		}
	}
}

func TestForeachOutputs(t *testing.T) {
	srv := usersAPI(t)
	res := run(t, srv, `
- id: empty
  use: create
  foreach: []
  params: { name: a, age: 1 }
  outputs:
    users: <<outputs.current.user>>
- use: http
  inputs:
    users: <<outputs.empty.users>>
  params: { server: api, method: GET, path: /api/users/1/followers }
  expects:
    - asserts:
        - inputs.users == []
`)
	if res.Status != Passed {
		t.Fatalf("status = %s: %v", res.Status, res.Steps)
	}
}
