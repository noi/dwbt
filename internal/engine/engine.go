// Package engine loads, validates and runs workflows. It is shared by the
// dwbt command and the dwbttest package.
package engine

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/action/httpaction"
	"github.com/noi/dwbt/internal/check"
	"github.com/noi/dwbt/internal/def"
	"github.com/noi/dwbt/internal/runner"
)

// Actions is a set of actions implemented in Go. Its methods return a new
// Actions and leave the receiver unchanged.
type Actions struct {
	m    map[string]action.Action
	errs []error
}

// Builtin returns the built-in actions.
func Builtin() Actions {
	return Actions{m: map[string]action.Action{"http": httpaction.New()}}
}

// With registers a under name, such as "db/seed". It cannot replace an
// action registered before. An invalid registration is reported by Map.
func (as Actions) With(name string, a action.Action) Actions {
	switch {
	case !def.IsActionName(name):
		return as.withError(fmt.Errorf("invalid action name %q", name))
	case as.m[name] != nil:
		return as.withError(fmt.Errorf("action %q is already registered", name))
	}
	m := maps.Clone(as.m)
	m[name] = a
	return Actions{m: m, errs: as.errs}
}

func (as Actions) withError(err error) Actions {
	return Actions{m: as.m, errs: append(slices.Clip(as.errs), err)}
}

// Map returns the actions by name, or the errors of invalid registrations.
func (as Actions) Map() (map[string]action.Action, error) {
	if len(as.errs) > 0 {
		return nil, def.Errors(as.errs)
	}
	return as.m, nil
}

// Plan is a loaded and validated set of workflows.
type Plan struct {
	Project   *def.Project
	Workflows []*def.Workflow
	Actions   map[string]action.Action
}

// Load loads the .dwbt directory dir and the workflow files, or all the
// workflows under dir/workflows when files is empty, and validates them.
// base is used to display file names.
//
// On error, the returned Plan is non-nil if the project could be read, so
// that its warnings can be reported.
func Load(dir, base string, files []string, actions map[string]action.Action) (*Plan, error) {
	proj, err := def.Load(dir, base)
	if proj == nil {
		return nil, err
	}
	p := &Plan{Project: proj, Actions: actions}
	errs := appendErr(nil, err)
	if len(files) == 0 {
		files = proj.WorkflowFiles
	}
	if len(files) == 0 && len(errs) == 0 {
		return p, fmt.Errorf("no workflows found in %s", filepath.Join(dir, "workflows"))
	}
	for _, f := range files {
		wf, err := proj.LoadWorkflow(f)
		errs = appendErr(errs, err)
		if wf != nil && err == nil {
			p.Workflows = append(p.Workflows, wf)
		}
	}
	if len(errs) == 0 {
		errs = appendErr(errs, check.Check(proj, actions, p.Workflows))
	}
	return p, errs.Err()
}

// RunOptions selects the servers the workflows run against.
type RunOptions struct {
	// Env is the environment profile; the default profile when empty.
	Env string
	// Servers overrides the URLs of servers by id.
	Servers map[string]string
	// Environ holds the environment variables, as from os.Environ.
	Environ []string
	// OverrideHint tells how to override the URL of server %[1]s, such as
	// "--server %[1]s=<url>".
	OverrideHint string
}

// Runner returns a runner of the workflows of p. A server given in
// opts.Servers takes precedence over the profile opts.Env, which takes
// precedence over the default profile.
func (p *Plan) Runner(opts RunOptions) (*runner.Runner, error) {
	env := environ(opts.Environ)
	rt, err := newRuntime(p.Project.Config, opts.Env, opts.Servers, env)
	if err != nil {
		return nil, err
	}
	rt.hint = opts.OverrideHint
	return &runner.Runner{
		Resolver: def.Resolver{Presets: p.Actions, Files: p.Project.Actions},
		Runtime:  rt,
		Env:      env,
	}, nil
}

func environ(kvs []string) map[string]any {
	env := map[string]any{}
	for _, kv := range kvs {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	return env
}

// Errors returns err as a list of errors.
func Errors(err error) []error {
	return appendErr(nil, err)
}

func appendErr(errs def.Errors, err error) def.Errors {
	if err == nil {
		return errs
	}
	var list def.Errors
	if errors.As(err, &list) {
		return append(errs, list...)
	}
	return append(errs, err)
}
