// Package studio implements the operations of the GUI on a .dwbt directory:
// listing definitions, editing them as documents, validating and running
// workflows.
package studio

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/gui/internal/doc"
	"github.com/noi/dwbt/internal/check"
	"github.com/noi/dwbt/internal/def"
	"github.com/noi/dwbt/internal/engine"
	"github.com/noi/dwbt/internal/runner"
	"github.com/noi/dwbt/internal/yamlx"
)

// Studio operates on one .dwbt directory at a time.
type Studio struct {
	presets map[string]action.Action
	environ []string

	mu  sync.Mutex
	dir string // the .dwbt directory; "" until a project is opened
	// cancel stops the running workflow, if any.
	cancel context.CancelFunc
}

// New returns a Studio using the given actions implemented in Go and the
// environment variables, as from os.Environ.
func New(presets map[string]action.Action, environ []string) *Studio {
	return &Studio{presets: presets, environ: environ}
}

// Project describes an opened .dwbt directory.
type Project struct {
	// Root is the directory containing .dwbt.
	Root string `json:"root"`
	// Workflows are the paths of the workflow files relative to
	// .dwbt/workflows, with slashes.
	Workflows []string `json:"workflows"`
	// Actions are the callable actions, sorted by name.
	Actions      []*ActionInfo `json:"actions"`
	Environments []string      `json:"environments"`
	DefaultEnv   string        `json:"defaultEnv"`
	// Servers are the ids of the servers of all the environments.
	Servers  []string   `json:"servers"`
	Warnings []string   `json:"warnings"`
	Problems []*Problem `json:"problems"`
}

// ActionInfo describes an action for the palette of the editor.
type ActionInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Builtin reports whether the action is implemented in Go.
	Builtin bool `json:"builtin"`
	// Kind is the default expectation type, such as "http".
	Kind    string       `json:"kind"`
	Params  []*ParamInfo `json:"params"`
	Outputs []string     `json:"outputs"`
}

// ParamInfo describes a parameter of an action.
type ParamInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

// Problem is an error found in the definitions.
type Problem struct {
	Message string `json:"message"`
	// File is the path of the file relative to the project root, or "".
	File string `json:"file"`
	Line int    `json:"line"`
	// Step is the index of the top-level step of the edited document the
	// problem is in, or -1.
	Step int `json:"step"`
}

// Open opens the .dwbt directory found in dir or its parents.
func (s *Studio) Open(dir string) (*Project, error) {
	root, err := def.FindRoot(dir)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.dir = root
	s.mu.Unlock()
	return s.Project()
}

// Project reloads the opened project.
func (s *Studio) Project() (*Project, error) {
	dir, err := s.root()
	if err != nil {
		return nil, err
	}
	proj, err := def.Load(dir, filepath.Dir(dir))
	if proj == nil {
		return nil, err
	}
	p := &Project{
		Root:       filepath.Dir(dir),
		Workflows:  []string{},
		Actions:    []*ActionInfo{},
		DefaultEnv: proj.Config.Default,
		Servers:    []string{},
		Warnings:   slices.Clip(proj.Warnings),
		Problems:   problems(engine.Errors(err), "", nil),
	}
	wfDir := filepath.Join(dir, "workflows")
	for _, f := range proj.WorkflowFiles {
		rel, _ := filepath.Rel(wfDir, f)
		p.Workflows = append(p.Workflows, filepath.ToSlash(rel))
	}
	p.Environments = slices.Sorted(maps.Keys(proj.Config.Environments))
	servers := map[string]bool{}
	for _, env := range proj.Config.Environments {
		for id := range env.Servers {
			servers[id] = true
		}
	}
	p.Servers = slices.Sorted(maps.Keys(servers))
	for name, a := range s.presets {
		info := &ActionInfo{Name: name, Builtin: true, Kind: action.KindOf(a), Outputs: slices.Clip(a.Outputs())}
		for _, pn := range slices.Sorted(maps.Keys(a.Params())) {
			ps := a.Params()[pn]
			info.Params = append(info.Params, &ParamInfo{Name: pn, Type: string(ps.Type), Required: ps.Required})
		}
		p.Actions = append(p.Actions, info)
	}
	for name, d := range proj.Actions {
		p.Actions = append(p.Actions, actionInfo(name, d))
	}
	slices.SortFunc(p.Actions, func(a, b *ActionInfo) int { return strings.Compare(a.Name, b.Name) })
	return p, nil
}

func actionInfo(name string, d *def.ActionDef) *ActionInfo {
	info := &ActionInfo{Name: name, Description: d.Description, Params: []*ParamInfo{}, Outputs: []string{}}
	for _, p := range d.Params {
		info.Params = append(info.Params, &ParamInfo{Name: p.Name, Type: string(p.Type), Required: true})
	}
	for _, o := range d.Outputs {
		info.Outputs = append(info.Outputs, o.Name)
	}
	return info
}

func (s *Studio) root() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == "" {
		return "", errors.New("no project is opened")
	}
	return s.dir, nil
}

// path returns the path of a definition file under the subdirectory sub of
// .dwbt, such as "workflows", rejecting names that escape it.
func (s *Studio) path(sub, name string) (string, error) {
	dir, err := s.root()
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if name == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid name %q", name)
	}
	return filepath.Join(dir, sub, clean), nil
}

// Workflow reads the workflow file name, relative to .dwbt/workflows.
func (s *Studio) Workflow(name string) (*doc.Workflow, error) {
	path, err := s.path("workflows", name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	wf, err := doc.ParseWorkflow(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return wf, nil
}

// SaveWorkflow writes wf to the workflow file name, relative to
// .dwbt/workflows. The file is created if needed.
func (s *Studio) SaveWorkflow(name string, wf *doc.Workflow) error {
	if !strings.HasSuffix(name, ".yaml") {
		return fmt.Errorf("workflow files must have the .yaml extension: %s", name)
	}
	data, err := doc.EncodeWorkflow(wf)
	if err != nil {
		return err
	}
	return s.write("workflows", name, data)
}

// Action reads the action file of the action name, such as "user/create".
func (s *Studio) Action(name string) (*doc.Action, error) {
	path, err := s.path("actions", name+".yaml")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	a, err := doc.ParseAction(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return a, nil
}

// SaveAction writes a to the action file of the action name. The file is
// created if needed.
func (s *Studio) SaveAction(name string, a *doc.Action) error {
	if !def.IsActionName(name) {
		return fmt.Errorf("invalid action name %q", name)
	}
	data, err := doc.EncodeAction(a)
	if err != nil {
		return err
	}
	return s.write("actions", name+".yaml", data)
}

func (s *Studio) write(sub, name string, data []byte) error {
	path, err := s.path(sub, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// RenderWorkflow returns the YAML of wf.
func (s *Studio) RenderWorkflow(wf *doc.Workflow) (string, error) {
	data, err := doc.EncodeWorkflow(wf)
	return string(data), err
}

// RenderAction returns the YAML of a.
func (s *Studio) RenderAction(a *doc.Action) (string, error) {
	data, err := doc.EncodeAction(a)
	return string(data), err
}

// CheckWorkflow validates wf as the workflow file name together with the
// actions of the project, without saving it.
func (s *Studio) CheckWorkflow(name string, wf *doc.Workflow) []*Problem {
	_, _, probs := s.loadWorkflow(name, wf)
	return probs
}

// loadWorkflow loads the project and wf as the workflow file name, and
// validates them.
func (s *Studio) loadWorkflow(name string, wf *doc.Workflow) (*def.Project, *def.Workflow, []*Problem) {
	path, err := s.path("workflows", name)
	if err != nil {
		return nil, nil, problems(engine.Errors(err), "", nil)
	}
	data, err := doc.EncodeWorkflow(wf)
	if err != nil {
		return nil, nil, problems(engine.Errors(err), "", nil)
	}
	dir, _ := s.root()
	proj, err := def.Load(dir, s.base())
	if proj == nil {
		return nil, nil, problems(engine.Errors(err), "", nil)
	}
	errs := engine.Errors(err)
	display := s.display(path)
	lines := stepLines(data)
	n, err := yamlx.Parse(data, display)
	if err != nil {
		return nil, nil, problems(append(errs, err), display, lines)
	}
	w, err := def.ParseWorkflow(n, display)
	if err != nil {
		return nil, nil, problems(append(errs, engine.Errors(err)...), display, lines)
	}
	w.Path = path
	if len(errs) == 0 {
		errs = engine.Errors(check.Check(proj, s.presets, []*def.Workflow{w}))
	}
	return proj, w, problems(errs, display, lines)
}

// CheckAction validates a as the action name together with the other
// actions of the project, without saving it.
func (s *Studio) CheckAction(name string, a *doc.Action) []*Problem {
	path, err := s.path("actions", name+".yaml")
	if err != nil {
		return problems(engine.Errors(err), "", nil)
	}
	if !def.IsActionName(name) {
		return problems([]error{fmt.Errorf("invalid action name %q", name)}, "", nil)
	}
	data, err := doc.EncodeAction(a)
	if err != nil {
		return problems(engine.Errors(err), "", nil)
	}
	dir, _ := s.root()
	display := s.display(path)
	lines := stepLines(data)
	proj, err := def.Load(dir, s.base())
	if proj == nil {
		return problems(engine.Errors(err), "", nil)
	}
	// Drop the errors of the saved version of the edited action.
	var errs []error
	for _, e := range engine.Errors(err) {
		if pos, ok := position(e); !ok || pos.File != display {
			errs = append(errs, e)
		}
	}
	n, err := yamlx.Parse(data, display)
	if err != nil {
		return problems(append(errs, err), display, lines)
	}
	d, err := def.ParseAction(n, name)
	if err != nil {
		return problems(append(errs, engine.Errors(err)...), display, lines)
	}
	proj.Actions[name] = d
	if len(errs) == 0 {
		errs = engine.Errors(check.Check(proj, s.presets, nil))
	}
	return problems(errs, display, lines)
}

// RunOptions selects the servers a workflow runs against.
type RunOptions struct {
	// Env is the environment profile; the default profile when empty.
	Env string `json:"env"`
	// Servers overrides the URLs of servers by id.
	Servers map[string]string `json:"servers"`
}

// RunResult is the result of running a workflow.
type RunResult struct {
	// Status is "ok", "FAIL" or "ERROR", or "" if the workflow could not
	// start because of Problems.
	Status   string        `json:"status"`
	Duration string        `json:"duration"`
	Steps    []*StepResult `json:"steps"`
	Problems []*Problem    `json:"problems"`
}

// StepResult is the result of a top-level step.
type StepResult struct {
	Index    int    `json:"index"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Duration string `json:"duration"`
	Error    string `json:"error"`
}

// Run runs wf as the workflow file name, without saving it. Only one
// workflow runs at a time.
func (s *Studio) Run(ctx context.Context, name string, wf *doc.Workflow, opts RunOptions) (*RunResult, error) {
	proj, w, probs := s.loadWorkflow(name, wf)
	if len(probs) > 0 {
		return &RunResult{Steps: []*StepResult{}, Problems: probs}, nil
	}
	plan := &engine.Plan{Project: proj, Workflows: []*def.Workflow{w}, Actions: s.presets}
	r, err := plan.Runner(engine.RunOptions{
		Env:          opts.Env,
		Servers:      opts.Servers,
		Environ:      s.environ,
		OverrideHint: "the server %[1]s in the run settings",
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return nil, errors.New("a workflow is already running")
	}
	s.cancel = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cancel = nil
		s.mu.Unlock()
	}()

	res := r.Run(ctx, w)
	out := &RunResult{
		Status:   res.Status.String(),
		Duration: engine.Duration(res.Duration),
		Steps:    []*StepResult{},
		Problems: []*Problem{},
	}
	for _, sr := range res.Steps {
		st := &StepResult{Index: sr.Step.Index, Label: sr.Step.Label(), Status: sr.Status.String()}
		if sr.Status != runner.Skipped {
			st.Duration = engine.Duration(sr.Duration)
		}
		if sr.Err != nil {
			st.Error = sr.Err.Error()
		}
		out.Steps = append(out.Steps, st)
	}
	return out, nil
}

// Stop stops the running workflow, if any.
func (s *Studio) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}

// base returns the directory file names are displayed relative to.
func (s *Studio) base() string {
	dir, _ := s.root()
	return filepath.Dir(dir)
}

func (s *Studio) display(path string) string {
	if rel, err := filepath.Rel(s.base(), path); err == nil {
		return rel
	}
	return path
}

// problems converts errors into problems. The positions in the file named
// display are mapped to the steps starting at lines.
func problems(errs []error, display string, lines []int) []*Problem {
	out := []*Problem{}
	for _, e := range errs {
		p := &Problem{Message: e.Error(), Step: -1}
		if pos, ok := position(e); ok {
			p.File, p.Line = pos.File, pos.Line
			if pos.File == display {
				p.Step = stepAt(lines, pos.Line)
			}
		}
		out = append(out, p)
	}
	return out
}

func position(err error) (yamlx.Pos, bool) {
	var ye *yamlx.Error
	if errors.As(err, &ye) {
		return ye.Pos, true
	}
	return yamlx.Pos{}, false
}

// stepLines returns the first line of each top-level step of the YAML data,
// followed by the line of the key after steps, if any.
func stepLines(data []byte) []int {
	var d yaml.Node
	if yaml.Unmarshal(data, &d) != nil || len(d.Content) == 0 {
		return nil
	}
	m := d.Content[0]
	var lines []int
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != "steps" {
			continue
		}
		for _, st := range m.Content[i+1].Content {
			lines = append(lines, st.Line)
		}
		if i+2 < len(m.Content) {
			lines = append(lines, m.Content[i+2].Line)
		} else {
			lines = append(lines, math.MaxInt)
		}
	}
	return lines
}

// stepAt returns the index of the step containing line, or -1.
func stepAt(lines []int, line int) int {
	for i := 0; i+1 < len(lines); i++ {
		if lines[i] <= line && line < lines[i+1] {
			return i
		}
	}
	return -1
}
