// Package runner executes workflows.
package runner

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/internal/def"
	"github.com/noi/dwbt/internal/tmpl"
	"github.com/noi/dwbt/internal/value"
	"github.com/noi/dwbt/internal/yamlx"
)

// Status is the outcome of a step or a workflow.
type Status int

const (
	Passed Status = iota
	// Failed means an expectation was not met.
	Failed
	// Errored means the step could not be executed.
	Errored
	Skipped
)

func (s Status) String() string {
	switch s {
	case Passed:
		return "ok"
	case Failed:
		return "FAIL"
	case Errored:
		return "ERROR"
	default:
		return "skip"
	}
}

// Failure is returned when an expectation is not met.
type Failure struct {
	Pos     yamlx.Pos
	Msg     string
	Details []string
}

func (f *Failure) Error() string {
	var b strings.Builder
	b.WriteString(f.Pos.String() + ": " + f.Msg)
	for _, d := range f.Details {
		b.WriteString("\n  " + d)
	}
	return b.String()
}

// IsFailure reports whether err is caused by an unmet expectation.
func IsFailure(err error) bool {
	var f *Failure
	return errors.As(err, &f)
}

// Section is the part of a workflow a step belongs to.
type Section int

const (
	Main Section = iota
	Setup
	Teardown
)

func (s Section) String() string {
	switch s {
	case Setup:
		return "setup"
	case Teardown:
		return "teardown"
	default:
		return "steps"
	}
}

// StepResult is the result of a top-level step.
type StepResult struct {
	Section  Section
	Step     *def.Step
	Status   Status
	Duration time.Duration
	Err      error
}

// Result is the result of a workflow.
type Result struct {
	Workflow *def.Workflow
	Status   Status
	Steps    []StepResult
	// Err is an error outside the steps, if any.
	Err      *BindingError
	Duration time.Duration
}

// BindingError is an error evaluating the outputs of the setup or the
// inputs of the workflow.
type BindingError struct {
	// In is "setup outputs" or "inputs".
	In string
	// At is the number of steps that ran before the error.
	At  int
	Err error
}

func (e *BindingError) Error() string { return e.In + ": " + e.Err.Error() }

func (e *BindingError) Unwrap() error { return e.Err }

// fail records status unless the workflow has already failed, so that the
// result of a workflow is that of its first failure.
func (r *Result) fail(status Status) {
	if r.Status == Passed {
		r.Status = status
	}
}

// Runner executes workflows.
type Runner struct {
	// Resolver resolves built-in actions and actions defined in files.
	Resolver def.Resolver
	Runtime  action.Runtime
	// Env holds the environment variables available as env.
	Env map[string]any
}

// Run executes the setup, the steps and the teardown of wf in order. Once
// a step fails, the remaining steps of the setup and the steps are skipped,
// but the teardown runs. A step of the teardown is skipped if it refers to
// inputs or outputs that are not available because of the failure.
func (r *Runner) Run(ctx context.Context, wf *def.Workflow) *Result {
	res := r.Resolver
	res.Locals = wf.Actions
	result := &Result{Workflow: wf}
	start := time.Now()

	inputs := map[string]any{}
	if wf.Setup != nil {
		published := r.steps(ctx, res, result, Setup, wf.Setup.Steps, nil, nil)
		env := tmpl.Env{"env": r.Env, "outputs": map[string]any{"steps": published}}
		setup := r.bindings(result, "setup outputs", wf.Setup.Outputs, env, "steps", published)
		env = tmpl.Env{"env": r.Env, "outputs": map[string]any{"setup": setup}}
		inputs = r.bindings(result, "inputs", wf.Inputs, env, "setup", setup)
	} else {
		inputs = r.bindings(result, "inputs", wf.Inputs, tmpl.Env{"env": r.Env}, "", nil)
	}
	published := r.steps(ctx, res, result, Main, wf.Steps, map[string]any{}, inputs)
	r.teardown(ctx, res, result, wf.Teardown, published, inputs)
	result.Duration = time.Since(start)
	return result
}

// steps executes steps in order, records their results and returns the
// outputs published by them. Once the workflow fails, the remaining steps
// are skipped. inputs are the inputs of the workflow.
func (r *Runner) steps(ctx context.Context, res def.Resolver, result *Result, sec Section, steps []*def.Step, published, inputs map[string]any) map[string]any {
	if published == nil {
		published = map[string]any{}
	}
	for _, st := range steps {
		if result.Status != Passed {
			result.Steps = append(result.Steps, StepResult{Section: sec, Step: st, Status: Skipped})
			continue
		}
		r.record(ctx, res, result, sec, st, published, inputs)
	}
	return published
}

// teardown executes the steps of the teardown, even after a failure, and
// skips those referring to unavailable inputs or outputs.
func (r *Runner) teardown(ctx context.Context, res def.Resolver, result *Result, steps []*def.Step, published, inputs map[string]any) {
	published = maps.Clone(published)
	for _, st := range steps {
		if !runnable(st, published, inputs) {
			result.Steps = append(result.Steps, StepResult{Section: Teardown, Step: st, Status: Skipped})
			continue
		}
		r.record(ctx, res, result, Teardown, st, published, inputs)
	}
}

// record executes a top-level step, records its result and publishes its
// outputs.
func (r *Runner) record(ctx context.Context, res def.Resolver, result *Result, sec Section, st *def.Step, published, inputs map[string]any) {
	t0 := time.Now()
	out, err := r.step(ctx, res, st, published, nil, inputs)
	sr := StepResult{Section: sec, Step: st, Duration: time.Since(t0), Err: err}
	switch {
	case err == nil:
		if st.ID != "" {
			published[st.ID] = out
		}
	case IsFailure(err):
		sr.Status = Failed
	default:
		sr.Status = Errored
	}
	result.fail(sr.Status)
	result.Steps = append(result.Steps, sr)
}

// bindings evaluates the outputs of the setup or the inputs of the
// workflow in env. A binding referring to outputs.<section>.<name> is
// skipped if name is not in available, since a step of the setup failed.
func (r *Runner) bindings(result *Result, in string, bs []*def.Binding, env tmpl.Env, section string, available map[string]any) map[string]any {
	out := map[string]any{}
	for _, b := range bs {
		if !refersTo(b.Value.Exprs(), func(ref tmpl.Ref) bool {
			if ref.Root != "outputs" || len(ref.Path) < 2 || ref.Path[0] != section {
				return true
			}
			_, ok := available[ref.Path[1]]
			return ok
		}) {
			continue
		}
		v, err := b.Value.Eval(env)
		if err != nil {
			if result.Err == nil {
				result.Err = &BindingError{In: in, At: len(result.Steps), Err: err}
			}
			result.fail(Errored)
			continue
		}
		out[b.Name] = v
	}
	return out
}

// runnable reports whether the inputs and outputs that st refers to are
// available: the outputs of the steps in published, and the inputs of the
// workflow in inputs.
func runnable(st *def.Step, published, inputs map[string]any) bool {
	for _, in := range st.Inputs {
		if !refersTo(in.Value.Exprs(), func(ref tmpl.Ref) bool {
			if ref.Root != "outputs" || len(ref.Path) == 0 {
				return true
			}
			_, ok := published[ref.Path[0]]
			return ok
		}) {
			return false
		}
	}
	var exprs []*tmpl.Expr
	if st.Foreach != nil {
		exprs = append(exprs, st.Foreach.Exprs()...)
	}
	if st.Params != nil {
		exprs = append(exprs, st.Params.Exprs()...)
	}
	for _, ex := range st.Expects {
		exprs = append(exprs, ex.Asserts...)
	}
	for _, o := range st.Outputs {
		exprs = append(exprs, o.Value.Exprs()...)
	}
	return refersTo(exprs, func(ref tmpl.Ref) bool {
		if ref.Root != "inputs" || len(ref.Path) == 0 {
			return true
		}
		for _, in := range st.Inputs {
			if in.Name == ref.Path[0] {
				return true
			}
		}
		_, ok := inputs[ref.Path[0]]
		return ok
	})
}

// refersTo reports whether ok holds for every reference of exprs.
func refersTo(exprs []*tmpl.Expr, ok func(tmpl.Ref) bool) bool {
	for _, e := range exprs {
		for _, ref := range e.Refs() {
			if !ok(ref) {
				return false
			}
		}
	}
	return true
}

// step executes a step and returns its outputs. published holds the outputs
// of the preceding steps, params the parameters of the enclosing action
// (nil at the top level of a workflow), and shared the inputs of the
// workflow, available to the step besides its own inputs.
func (r *Runner) step(ctx context.Context, res def.Resolver, st *def.Step, published, params, shared map[string]any) (map[string]any, error) {
	callee, ok := res.Resolve(st.Use)
	if !ok {
		return nil, yamlx.Errorf(st.UsePos, "unknown action %q", st.Use)
	}
	base := tmpl.Env{"env": r.Env}
	if params != nil {
		base["params"] = params
	}
	inputs := maps.Clone(shared)
	if inputs == nil {
		inputs = map[string]any{}
	}
	inEnv := base.With("outputs", published)
	for _, in := range st.Inputs {
		v, err := in.Value.Eval(inEnv)
		if err != nil {
			return nil, err
		}
		inputs[in.Name] = v
	}
	env := base.With("inputs", inputs)

	if st.Foreach == nil {
		return r.iteration(ctx, res, st, callee, env)
	}
	v, err := st.Foreach.Eval(env)
	if err != nil {
		return nil, err
	}
	items, ok := v.([]any)
	if !ok {
		return nil, yamlx.Errorf(st.Foreach.Position(), "foreach must be an array, got %s", value.TypeName(v))
	}
	outputs := map[string]any{}
	for _, o := range st.Outputs {
		outputs[o.Name] = []any{}
	}
	for i, item := range items {
		out, err := r.iteration(ctx, res, st, callee, env.With("item", item).With("index", i))
		if err != nil {
			return nil, fmt.Errorf("foreach[%d]: %w", i, err)
		}
		for k, v := range out {
			outputs[k] = append(outputs[k].([]any), v)
		}
	}
	return outputs, nil
}

// iteration calls the action of a step once, checks the expectations and
// evaluates the outputs.
func (r *Runner) iteration(ctx context.Context, res def.Resolver, st *def.Step, callee def.Callee, env tmpl.Env) (map[string]any, error) {
	args := map[string]any{}
	if st.Params != nil {
		v, err := st.Params.Eval(env)
		if err != nil {
			return nil, err
		}
		args = v.(map[string]any)
	}
	current, err := r.call(ctx, res, callee, args, st.Pos)
	if err != nil {
		return nil, err
	}
	env = env.With("outputs", map[string]any{"current": current})
	if err := expects(st, callee, current, env); err != nil {
		return nil, err
	}
	outputs := map[string]any{}
	for _, o := range st.Outputs {
		v, err := o.Value.Eval(env)
		if err != nil {
			return nil, err
		}
		outputs[o.Name] = v
	}
	return outputs, nil
}

// call executes an action with evaluated arguments and returns its outputs.
func (r *Runner) call(ctx context.Context, res def.Resolver, callee def.Callee, args map[string]any, pos yamlx.Pos) (map[string]any, error) {
	for name, p := range callee.ParamSpec() {
		v, ok := args[name]
		if !ok {
			if p.Required {
				return nil, yamlx.Errorf(pos, "missing parameter %q for action %q", name, callee.Name)
			}
			continue
		}
		if !p.Type.Accepts(v) {
			return nil, yamlx.Errorf(pos, "parameter %q of action %q must be %s, got %s", name, callee.Name, p.Type, value.TypeName(v))
		}
	}

	if callee.Go != nil {
		out, err := callee.Go.Run(ctx, r.Runtime, args)
		if err != nil {
			return nil, yamlx.Errorf(pos, "%s: %v", callee.Name, err)
		}
		if out == nil {
			out = map[string]any{}
		}
		return out, nil
	}

	d := callee.Def
	inner := res.For(d)
	published := map[string]any{}
	for _, st := range d.Steps {
		out, err := r.step(ctx, inner, st, published, args, nil)
		if err != nil {
			return nil, fmt.Errorf("in action %s, step %s: %w", d.Name, st.Label(), err)
		}
		if st.ID != "" {
			published[st.ID] = out
		}
	}
	env := tmpl.Env{"env": r.Env, "params": args, "outputs": map[string]any{"steps": published}}
	outputs := map[string]any{}
	for _, o := range d.Outputs {
		v, err := o.Value.Eval(env)
		if err != nil {
			return nil, fmt.Errorf("in action %s: %w", d.Name, err)
		}
		outputs[o.Name] = v
	}
	return outputs, nil
}

// expects checks the expectations of a step in order, and stops at the
// first one that is not met.
func expects(st *def.Step, callee def.Callee, current map[string]any, env tmpl.Env) error {
	kind := callee.Kind()
	for i, ex := range st.Expects {
		typ := kind
		if ex.Type != "" {
			typ = ex.Type
		}
		if ex.Status != nil {
			if typ != "http" {
				return yamlx.Errorf(ex.StatusPos, "status is only available for http expectations")
			}
			res, _ := current["res"].(map[string]any)
			got := res["status"]
			if !numEqual(got, *ex.Status) {
				return &Failure{
					Pos:     ex.StatusPos,
					Msg:     fmt.Sprintf("expects[%d].status: expected %d, got %s", i, *ex.Status, value.Format(got)),
					Details: responseDetails(res),
				}
			}
		}
		for j, a := range ex.Asserts {
			v, err := a.Eval(env)
			if err != nil {
				return err
			}
			ok, isBool := v.(bool)
			if !isBool {
				return yamlx.Errorf(a.Pos, "assert must evaluate to bool, got %s: %s", value.TypeName(v), a.Source)
			}
			if !ok {
				return &Failure{
					Pos:     a.Pos,
					Msg:     fmt.Sprintf("expects[%d].asserts[%d] failed: %s", i, j, a.Source),
					Details: a.Explain(env),
				}
			}
		}
	}
	return nil
}

func numEqual(v any, want int) bool {
	f, ok := value.ToFloat(v)
	return ok && f == float64(want)
}

const maxDetail = 500

func responseDetails(res map[string]any) []string {
	var lines []string
	if req, ok := res["req"].(map[string]any); ok {
		lines = append(lines, fmt.Sprintf("res.req: %v %v", req["method"], req["url"]))
	}
	if body, ok := res["body"]; ok {
		s := value.Format(body)
		if len(s) > maxDetail {
			s = s[:maxDetail] + "..."
		}
		lines = append(lines, "res.body: "+s)
	}
	return lines
}
