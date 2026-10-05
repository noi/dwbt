// Package dwbttest runs dwbt workflows from Go tests.
//
//	func TestE2E(t *testing.T) {
//		srv := httptest.NewServer(app.NewHandler())
//		defer srv.Close()
//
//		dwbttest.New("testdata/.dwbt").
//			Server("api", srv.URL).
//			Action("db/seed", seedAction{}).
//			Run(t)
//	}
//
// Each workflow runs as a subtest named after its path under the workflows
// directory, so a single workflow can be selected with
// go test -run TestE2E/follow.yaml.
package dwbttest

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/internal/engine"
	"github.com/noi/dwbt/internal/runner"
)

// Suite runs the workflows of a .dwbt directory. Its methods return a new
// Suite and leave the receiver unchanged, so a Suite can be shared by tests
// and extended for each of them.
type Suite struct {
	dir       string
	env       string
	servers   map[string]string
	actions   engine.Actions
	workflows []string
}

// New returns a Suite running the workflows of the .dwbt directory dir,
// such as "testdata/.dwbt". Unlike the dwbt command, it does not search the
// parent directories.
func New(dir string) Suite {
	return Suite{dir: dir, actions: engine.Builtin()}
}

// Server overrides the URL of the server id, like --server id=url of the dwbt
// command.
func (s Suite) Server(id, url string) Suite {
	servers := maps.Clone(s.servers)
	if servers == nil {
		servers = map[string]string{}
	}
	servers[id] = url
	s.servers = servers
	return s
}

// Env selects the environment profile of config.yaml, like --env of the dwbt
// command.
func (s Suite) Env(name string) Suite {
	s.env = name
	return s
}

// Action registers an action implemented in Go under name, such as
// "db/seed". It cannot replace a built-in action or an action registered
// before.
func (s Suite) Action(name string, a action.Action) Suite {
	s.actions = s.actions.With(name, a)
	return s
}

// Workflows limits the workflows to run to paths, which are relative to the
// workflows directory, such as "follow.yaml". All the workflows run when it
// is not called. Paths given by multiple calls are combined.
func (s Suite) Workflows(paths ...string) Suite {
	s.workflows = append(slices.Clip(s.workflows), paths...)
	return s
}

// Run validates the definitions and runs each workflow as a subtest of t.
// Invalid definitions fail t without running any workflow.
func (s Suite) Run(t *testing.T) {
	t.Helper()
	actions, err := s.actions.Map()
	if err != nil {
		fatal(t, err)
	}
	dir, err := filepath.Abs(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("%s is not a directory", s.dir)
	}
	// File names are displayed relative to the current directory, the
	// directory of the package under test, or to the parent of dir if dir is
	// outside of it.
	base := filepath.Dir(dir)
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, dir); err == nil && filepath.IsLocal(rel) {
			base = wd
		}
	}
	wfDir := filepath.Join(dir, "workflows")
	var files []string
	for _, p := range s.workflows {
		if !filepath.IsAbs(p) {
			p = filepath.Join(wfDir, p)
		}
		files = append(files, filepath.Clean(p))
	}

	plan, err := engine.Load(dir, base, files, actions)
	if plan != nil {
		for _, w := range plan.Project.Warnings {
			t.Logf("warning: %s", w)
		}
	}
	if err != nil {
		fatal(t, err)
	}
	r, err := plan.Runner(engine.RunOptions{
		Env:          s.env,
		Servers:      s.servers,
		Environ:      os.Environ(),
		OverrideHint: "Server(%[1]q, url)",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, wf := range plan.Workflows {
		name := wf.Name
		if rel, err := filepath.Rel(wfDir, wf.Path); err == nil && !strings.HasPrefix(rel, "..") {
			name = filepath.ToSlash(rel)
		}
		t.Run(name, func(t *testing.T) {
			res := r.Run(t.Context(), wf)
			title := wf.Name
			if wf.Description != "" {
				title += " - " + wf.Description
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%s %s (%s)\n", res.Status, title, engine.Duration(res.Duration))
			engine.WriteSteps(&b, res)
			msg := strings.TrimSuffix(b.String(), "\n")
			if res.Status == runner.Passed {
				t.Log(msg)
			} else {
				t.Error(msg)
			}
		})
	}
}

func fatal(t *testing.T, err error) {
	t.Helper()
	for _, e := range engine.Errors(err) {
		t.Errorf("error: %v", e)
	}
	t.FailNow()
}
