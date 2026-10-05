package studio

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/noi/dwbt/examples/users/mockapi"
	"github.com/noi/dwbt/gui/internal/doc"
	"github.com/noi/dwbt/internal/engine"
)

// open copies the users example into a temporary directory and opens it.
func open(t *testing.T) (*Studio, *Project) {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(filepath.Join(dir, ".dwbt"), os.DirFS("../../../examples/users/.dwbt")); err != nil {
		t.Fatal(err)
	}
	presets, err := engine.Builtin().Map()
	if err != nil {
		t.Fatal(err)
	}
	s := New(presets, nil)
	p, err := s.Open(filepath.Join(dir, ".dwbt", "workflows"))
	if err != nil {
		t.Fatal(err)
	}
	return s, p
}

func TestProject(t *testing.T) {
	_, p := open(t)
	if want := []string{"follow.yaml", "validation.yaml"}; !slices.Equal(p.Workflows, want) {
		t.Errorf("workflows: got %v, want %v", p.Workflows, want)
	}
	var names []string
	for _, a := range p.Actions {
		names = append(names, a.Name)
	}
	if want := []string{"http", "user/create", "user/delete", "user/follow"}; !slices.Equal(names, want) {
		t.Errorf("actions: got %v, want %v", names, want)
	}
	if http := p.Actions[0]; !http.Builtin || http.Kind != "http" || !slices.Equal(http.Outputs, []string{"res"}) {
		t.Errorf("http: got %+v", http)
	}
	if create := p.Actions[1]; create.Builtin || !slices.Equal(create.Outputs, []string{"user"}) || len(create.Params) != 2 {
		t.Errorf("user/create: got %+v", create)
	}
	if !slices.Equal(p.Environments, []string{"ci", "local"}) || p.DefaultEnv != "local" || !slices.Equal(p.Servers, []string{"api"}) {
		t.Errorf("config: got %v %q %v", p.Environments, p.DefaultEnv, p.Servers)
	}
	if len(p.Problems) != 0 {
		t.Errorf("problems: %+v", p.Problems)
	}
}

func TestCheckWorkflow(t *testing.T) {
	s, _ := open(t)
	wf, err := s.Workflow("follow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if probs := s.CheckWorkflow("follow.yaml", wf); len(probs) != 0 {
		t.Errorf("valid workflow: %+v", probs[0])
	}

	wf.Steps[0].Use = "user/unfollow"
	wf.Steps[1].Params = append(wf.Steps[1].Params, &doc.Binding{Name: "x", Value: "<<outputs.nope>>"})
	probs := s.CheckWorkflow("follow.yaml", wf)
	if len(probs) != 3 {
		t.Fatalf("got %d problems", len(probs))
	}
	if p := probs[0]; p.Section != "steps" || p.Step != 0 || p.File != filepath.Join(".dwbt", "workflows", "follow.yaml") || !strings.Contains(p.Message, `unknown action "user/unfollow"`) {
		t.Errorf("got %+v", p)
	}
	for _, p := range probs[1:] {
		if p.Section != "steps" || p.Step != 1 {
			t.Errorf("got %+v", p)
		}
	}
}

func TestCheckWorkflowSections(t *testing.T) {
	s, _ := open(t)
	http := func(path string) *doc.Step {
		return &doc.Step{Use: "http", Params: []*doc.Binding{{Name: "server", Value: "api"}, {Name: "method", Value: "GET"}, {Name: "path", Value: path}}}
	}
	wf := &doc.Workflow{
		Setup:    &doc.Setup{Steps: []*doc.Step{http("/"), {Use: "nope"}}},
		Inputs:   []*doc.Binding{{Name: "x", Value: "1"}},
		Steps:    []*doc.Step{http("/<<inputs.y>>")},
		Teardown: []*doc.Step{http("/"), http("/<<inputs.z>>")},
	}
	var got []string
	for _, p := range s.CheckWorkflow("new.yaml", wf) {
		got = append(got, fmt.Sprintf("%s %d", p.Section, p.Step))
	}
	if want := []string{"setup 1", "steps 0", "teardown 1"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCheckAction(t *testing.T) {
	s, _ := open(t)
	a, err := s.Action("user/create")
	if err != nil {
		t.Fatal(err)
	}
	if probs := s.CheckAction("user/create", a); len(probs) != 0 {
		t.Errorf("valid action: %+v", probs[0])
	}
	a.Outputs[0].Value = "<<outputs.steps.nope.user>>"
	probs := s.CheckAction("user/create", a)
	if len(probs) != 1 || probs[0].Step != -1 || probs[0].Line == 0 {
		t.Errorf("got %+v", probs)
	}
}

func TestSave(t *testing.T) {
	s, _ := open(t)
	wf := &doc.Workflow{Description: "new", Steps: []*doc.Step{{Use: "user/follow", Params: []*doc.Binding{{Name: "follower", Value: "1"}, {Name: "followee", Value: "2"}}}}}
	if err := s.SaveWorkflow("sub/new.yaml", wf); err != nil {
		t.Fatal(err)
	}
	a := &doc.Action{Steps: []*doc.Step{{Use: "http"}}}
	if err := s.SaveAction("user/delete", a); err != nil {
		t.Fatal(err)
	}
	p, err := s.Project()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(p.Workflows, "sub/new.yaml") {
		t.Errorf("workflows: %v", p.Workflows)
	}
	if got, err := s.Workflow("sub/new.yaml"); err != nil || got.Description != "new" {
		t.Errorf("got %+v, %v", got, err)
	}

	for _, name := range []string{"../x.yaml", "/abs.yaml", "x.yml", ""} {
		if err := s.SaveWorkflow(name, wf); err == nil {
			t.Errorf("SaveWorkflow(%q) succeeded", name)
		}
	}
	if err := s.SaveAction("../x", a); err == nil {
		t.Error("SaveAction(../x) succeeded")
	}
}

func TestRun(t *testing.T) {
	srv := httptest.NewServer(mockapi.New())
	defer srv.Close()
	s, _ := open(t)
	wf, err := s.Workflow("follow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	opts := RunOptions{Servers: map[string]string{"api": srv.URL}}
	res, err := s.Run(context.Background(), "follow.yaml", wf, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "ok" || len(res.Steps) != 4 || res.Steps[0].Section != "setup" || res.Steps[0].Label != "create (user/create)" || res.Steps[3].Section != "teardown" {
		t.Errorf("got %+v", res)
	}

	status := 500
	wf.Steps[1].Expects[0].Status = &status
	res, err = s.Run(context.Background(), "follow.yaml", wf, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "FAIL" || res.Steps[2].Status != "FAIL" || !strings.Contains(res.Steps[2].Error, "expected 500, got 200") {
		t.Errorf("got %+v", res.Steps[2])
	}
	// The teardown runs after the failure.
	if res.Steps[3].Status != "ok" {
		t.Errorf("teardown: got %+v", res.Steps[3])
	}

	wf.Steps[0].Use = "nope"
	res, err = s.Run(context.Background(), "follow.yaml", wf, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "" || len(res.Problems) == 0 {
		t.Errorf("got %+v", res)
	}
}
