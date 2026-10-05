// Package check validates definitions before they run: name resolution,
// parameters, expectation types, cycles and the scope of every expression.
package check

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/internal/def"
	"github.com/noi/dwbt/internal/tmpl"
	"github.com/noi/dwbt/internal/yamlx"
)

// Check validates the actions of the project and the given workflows.
func Check(p *def.Project, presets map[string]action.Action, workflows []*def.Workflow) error {
	c := &checker{}
	files := def.Resolver{Presets: presets, Files: p.Actions}

	for _, name := range slices.Sorted(maps.Keys(p.Actions)) {
		d := p.Actions[name]
		if _, ok := presets[name]; ok {
			c.errorf(d.Pos, "action %q conflicts with a built-in action", name)
		}
		c.action(d, files)
	}
	c.cycles(slices.Collect(maps.Values(p.Actions)), files)

	for _, wf := range workflows {
		res := files
		res.Locals = wf.Actions
		for _, name := range slices.Sorted(maps.Keys(wf.Actions)) {
			d := wf.Actions[name]
			if _, ok := presets[name]; ok {
				c.errorf(d.Pos, "action %q conflicts with a built-in action", name)
			} else if _, ok := p.Actions[name]; ok {
				c.errorf(d.Pos, "action %q is already defined in the actions directory", name)
			}
			c.action(d, res)
		}
		c.cycles(slices.Collect(maps.Values(wf.Actions)), res)
		c.steps(wf.Steps, scope{res: res})
	}
	return c.errs.Err()
}

type checker struct {
	errs def.Errors
}

func (c *checker) errorf(pos yamlx.Pos, format string, args ...any) {
	c.errs = append(c.errs, yamlx.Errorf(pos, format, args...))
}

// scope describes where a list of steps is defined.
type scope struct {
	res def.Resolver
	// params are the parameters of the enclosing action; nil at the top
	// level of a workflow.
	params []string
}

func (c *checker) action(d *def.ActionDef, res def.Resolver) {
	sc := scope{res: res.For(d), params: []string{}}
	for _, p := range d.Params {
		sc.params = append(sc.params, p.Name)
	}
	published := c.steps(d.Steps, sc)
	for _, o := range d.Outputs {
		c.refs(o.Value.Exprs(), rules{
			where:  "action outputs",
			params: sc.params,
			steps:  published,
		})
	}
}

// steps checks a list of steps and returns the outputs published by each
// step id.
func (c *checker) steps(steps []*def.Step, sc scope) map[string][]string {
	published := map[string][]string{}
	for _, st := range steps {
		callee, ok := sc.res.Resolve(st.Use)
		if !ok && st.Use != "" {
			c.errorf(st.UsePos, "unknown action %q", st.Use)
		}
		var inputs []string
		for _, in := range st.Inputs {
			inputs = append(inputs, in.Name)
		}
		var currentKeys []string
		if ok {
			currentKeys = callee.OutputNames()
			c.params(st, callee)
			c.expects(st, callee)
		}

		for _, in := range st.Inputs {
			c.refs(in.Value.Exprs(), rules{
				where:     "inputs",
				params:    sc.params,
				published: published,
			})
		}
		if st.Foreach != nil {
			c.refs(st.Foreach.Exprs(), rules{where: "foreach", params: sc.params, inputs: inputs})
		}
		item := st.Foreach != nil
		if st.Params != nil {
			c.refs(st.Params.Exprs(), rules{where: "params", params: sc.params, inputs: inputs, item: item})
		}
		after := rules{params: sc.params, inputs: inputs, item: item, current: true, currentKeys: currentKeys, known: ok}
		for _, ex := range st.Expects {
			after.where = "expects"
			c.refs(ex.Asserts, after)
		}
		for _, o := range st.Outputs {
			after.where = "outputs"
			c.refs(o.Value.Exprs(), after)
		}

		if st.ID == "" {
			if len(st.Outputs) > 0 {
				c.errorf(st.Pos, "a step with outputs must have an id")
			}
			continue
		}
		if st.ID == "current" || st.ID == "steps" {
			c.errorf(st.IDPos, "step id %q is reserved", st.ID)
			continue
		}
		if _, dup := published[st.ID]; dup {
			c.errorf(st.IDPos, "duplicate step id %q", st.ID)
			continue
		}
		names := []string{}
		for _, o := range st.Outputs {
			names = append(names, o.Name)
		}
		published[st.ID] = names
	}
	return published
}

func (c *checker) params(st *def.Step, callee def.Callee) {
	spec := callee.ParamSpec()
	given := map[string]bool{}
	if st.Params != nil {
		for i, k := range st.Params.Keys {
			given[k] = true
			p, ok := spec[k]
			if !ok {
				c.errorf(st.Params.KeyPos[i], "action %q has no parameter %q", callee.Name, k)
				continue
			}
			v := st.Params.Values[i]
			if len(v.Exprs()) > 0 {
				continue
			}
			lit, _ := v.Eval(nil)
			if !p.Type.Accepts(lit) {
				c.errorf(v.Position(), "parameter %q must be %s", k, p.Type)
			}
		}
	}
	var missing []string
	for _, name := range slices.Sorted(maps.Keys(spec)) {
		if spec[name].Required && !given[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		c.errorf(st.Pos, "missing parameters for action %q: %s", callee.Name, strings.Join(missing, ", "))
	}
}

// expectTypes are the known expectation types besides the generic one.
var expectTypes = []string{"http"}

func (c *checker) expects(st *def.Step, callee def.Callee) {
	kind := callee.Kind()
	for _, ex := range st.Expects {
		typ := kind
		if ex.Type != "" {
			if !slices.Contains(expectTypes, ex.Type) {
				c.errorf(ex.TypePos, "unknown expectation type %q", ex.Type)
				continue
			}
			if ex.Type != kind {
				c.errorf(ex.TypePos, "expectation type %q is not available for action %q", ex.Type, callee.Name)
				continue
			}
			typ = ex.Type
		}
		if ex.Status != nil && typ != "http" {
			c.errorf(ex.StatusPos, "status is only available for http expectations")
		}
	}
}

// rules tells which variables an expression may reference.
type rules struct {
	where string
	// params are the parameters of the enclosing action, or nil outside
	// actions.
	params []string
	// inputs are the inputs of the step, or nil if inputs are not available.
	inputs []string
	// item reports whether item and index are available.
	item bool
	// published holds the outputs of the preceding steps, available only in
	// inputs.
	published map[string][]string
	// current reports whether outputs.current is available, and
	// currentKeys holds its keys if known is set.
	current     bool
	currentKeys []string
	known       bool
	// steps is set in action outputs, where outputs.steps holds the outputs
	// of the action's steps.
	steps map[string][]string
}

func (c *checker) refs(exprs []*tmpl.Expr, r rules) {
	for _, e := range exprs {
		for _, ref := range e.Refs() {
			if msg := r.check(ref); msg != "" {
				c.errorf(e.Pos, "%s: %s", ref.Source, msg)
			}
		}
	}
}

func (r rules) check(ref tmpl.Ref) string {
	path := ref.Path
	switch ref.Root {
	case "env":
		return ""
	case "params":
		if r.params == nil {
			return "params can only be referenced inside an action"
		}
		if len(path) > 0 && !slices.Contains(r.params, path[0]) {
			return fmt.Sprintf("undeclared parameter %q", path[0])
		}
	case "inputs":
		if r.inputs == nil && r.where == "inputs" {
			return "inputs cannot be referenced in inputs"
		}
		if len(path) > 0 && !slices.Contains(r.inputs, path[0]) {
			return fmt.Sprintf("undeclared input %q", path[0])
		}
	case "item", "index":
		if !r.item {
			return ref.Root + " can only be used in params, expects and outputs of a step with foreach"
		}
	case "outputs":
		if len(path) == 0 {
			return "outputs must be followed by current, steps or a step id"
		}
		switch path[0] {
		case "current":
			return r.checkCurrent(path[1:])
		case "steps":
			return r.checkSteps(path[1:])
		}
		if r.published == nil {
			return "outputs of other steps can only be received via inputs"
		}
		keys, ok := r.published[path[0]]
		if !ok {
			return fmt.Sprintf("no step with id %q and outputs precedes this step", path[0])
		}
		if len(path) > 1 && !slices.Contains(keys, path[1]) {
			return fmt.Sprintf("step %q does not publish output %q", path[0], path[1])
		}
	default:
		return fmt.Sprintf("unknown variable %q", ref.Root)
	}
	return ""
}

func (r rules) checkCurrent(path []string) string {
	if !r.current {
		msg := "outputs.current is not available in " + r.where
		if r.steps != nil {
			msg += "; use outputs.steps.<id> for the outputs of the action's steps"
		}
		return msg
	}
	if r.known && len(path) > 0 && !slices.Contains(r.currentKeys, path[0]) {
		return fmt.Sprintf("the action does not publish output %q", path[0])
	}
	return ""
}

func (r rules) checkSteps(path []string) string {
	if r.steps == nil {
		return "outputs.steps is only available in action outputs"
	}
	if len(path) == 0 {
		return ""
	}
	keys, ok := r.steps[path[0]]
	if !ok {
		return fmt.Sprintf("the action has no step with id %q and outputs", path[0])
	}
	if len(path) > 1 && !slices.Contains(keys, path[1]) {
		return fmt.Sprintf("step %q does not publish output %q", path[0], path[1])
	}
	return ""
}

// cycles reports user-defined actions that call each other in a loop.
func (c *checker) cycles(defs []*def.ActionDef, res def.Resolver) {
	slices.SortFunc(defs, func(a, b *def.ActionDef) int { return strings.Compare(a.Name, b.Name) })
	const (
		visiting = 1
		done     = 2
	)
	state := map[*def.ActionDef]int{}
	var stack []string
	var visit func(d *def.ActionDef)
	visit = func(d *def.ActionDef) {
		switch state[d] {
		case done:
			return
		case visiting:
			i := slices.Index(stack, d.Name)
			c.errorf(d.Pos, "actions call each other in a cycle: %s -> %s", strings.Join(stack[i:], " -> "), d.Name)
			return
		}
		state[d] = visiting
		stack = append(stack, d.Name)
		inner := res.For(d)
		for _, st := range d.Steps {
			if callee, ok := inner.Resolve(st.Use); ok && callee.Def != nil {
				visit(callee.Def)
			}
		}
		stack = stack[:len(stack)-1]
		state[d] = done
	}
	for _, d := range defs {
		visit(d)
	}
}
