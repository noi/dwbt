package def

import (
	"github.com/noi/dwbt/action"
)

// Callee is the action a step refers to with use.
type Callee struct {
	Name string
	// Def is set for user-defined actions.
	Def *ActionDef
	// Go is set for actions implemented in Go.
	Go action.Action
}

// Kind returns the default expectation type for steps using the callee.
func (c Callee) Kind() string {
	if c.Go != nil {
		return action.KindOf(c.Go)
	}
	return ""
}

// OutputNames returns the names of the outputs the callee publishes.
func (c Callee) OutputNames() []string {
	if c.Go != nil {
		return c.Go.Outputs()
	}
	names := make([]string, len(c.Def.Outputs))
	for i, o := range c.Def.Outputs {
		names[i] = o.Name
	}
	return names
}

// ParamSpec returns the parameters of the callee. All parameters of
// user-defined actions are required.
func (c Callee) ParamSpec() action.ParamSpec {
	if c.Go != nil {
		return c.Go.Params()
	}
	spec := action.ParamSpec{}
	for _, p := range c.Def.Params {
		spec[p.Name] = action.Param{Type: p.Type, Required: true}
	}
	return spec
}

// Resolver resolves action names.
type Resolver struct {
	Presets map[string]action.Action
	Files   map[string]*ActionDef
	// Locals are the actions defined in the current workflow file.
	Locals map[string]*ActionDef
}

// Resolve looks up the action with the given name.
func (r Resolver) Resolve(name string) (Callee, bool) {
	if a, ok := r.Presets[name]; ok {
		return Callee{Name: name, Go: a}, true
	}
	if d, ok := r.Locals[name]; ok {
		return Callee{Name: name, Def: d}, true
	}
	if d, ok := r.Files[name]; ok {
		return Callee{Name: name, Def: d}, true
	}
	return Callee{}, false
}

// For returns the resolver used inside the given action. Actions defined
// in files cannot see the actions local to a workflow.
func (r Resolver) For(d *ActionDef) Resolver {
	if d.Local {
		return r
	}
	return Resolver{Presets: r.Presets, Files: r.Files}
}
