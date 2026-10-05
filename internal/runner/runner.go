// Package runner executes workflows.
package runner

import (
	"context"
	"errors"
	"fmt"
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

// StepResult is the result of a top-level step.
type StepResult struct {
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
	Duration time.Duration
}

// Runner executes workflows.
type Runner struct {
	// Resolver resolves built-in actions and actions defined in files.
	Resolver def.Resolver
	Runtime  action.Runtime
	// Env holds the environment variables available as env.
	Env map[string]any
}

// Run executes the steps of wf in order. Once a step fails, the remaining
// steps are skipped.
func (r *Runner) Run(ctx context.Context, wf *def.Workflow) *Result {
	res := r.Resolver
	res.Locals = wf.Actions
	result := &Result{Workflow: wf}
	start := time.Now()
	published := map[string]any{}
	for _, st := range wf.Steps {
		if result.Status != Passed {
			result.Steps = append(result.Steps, StepResult{Step: st, Status: Skipped})
			continue
		}
		t0 := time.Now()
		out, err := r.step(ctx, res, st, published, nil)
		sr := StepResult{Step: st, Duration: time.Since(t0), Err: err}
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
		result.Status = sr.Status
		result.Steps = append(result.Steps, sr)
	}
	result.Duration = time.Since(start)
	return result
}

// step executes a step and returns its outputs. published holds the outputs
// of the preceding steps, and params the parameters of the enclosing action
// (nil at the top level of a workflow).
func (r *Runner) step(ctx context.Context, res def.Resolver, st *def.Step, published, params map[string]any) (map[string]any, error) {
	callee, ok := res.Resolve(st.Use)
	if !ok {
		return nil, yamlx.Errorf(st.UsePos, "unknown action %q", st.Use)
	}
	base := tmpl.Env{"env": r.Env}
	if params != nil {
		base["params"] = params
	}
	inputs := map[string]any{}
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
		out, err := r.step(ctx, inner, st, published, args)
		if err != nil {
			return nil, fmt.Errorf("in action %s, step %s: %w", d.Name, st.Label(), err)
		}
		if st.ID != "" {
			published[st.ID] = out
		}
	}
	env := tmpl.Env{"env": r.Env, "params": args, "outputs": map[string]any{"current": published}}
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
