package def

import (
	"slices"
	"strings"
	"time"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/internal/tmpl"
	"github.com/noi/dwbt/internal/yamlx"
)

// parser accumulates errors so that as many as possible are reported at once.
type parser struct {
	errs Errors
}

func (p *parser) errorf(pos yamlx.Pos, format string, args ...any) {
	p.errs = append(p.errs, yamlx.Errorf(pos, format, args...))
}

func (p *parser) add(err error) {
	if err != nil {
		p.errs = append(p.errs, err)
	}
}

// mapping checks that n is a mapping with only the allowed keys.
func (p *parser) mapping(n *yamlx.Node, what string, allowed ...string) bool {
	if n.Kind != yamlx.Map {
		p.errorf(n.Pos, "%s must be a mapping, got %s", what, n.Kind)
		return false
	}
	for _, pair := range n.Pairs {
		if !slices.Contains(allowed, pair.Key) {
			p.errorf(pair.KeyPos, "unknown key %q in %s (allowed: %s)", pair.Key, what, strings.Join(allowed, ", "))
		}
	}
	return true
}

func (p *parser) str(n *yamlx.Node, what string) (string, bool) {
	if n == nil {
		return "", false
	}
	s, ok := n.Value.(string)
	if n.Kind != yamlx.Scalar || !ok {
		p.errorf(n.Pos, "%s must be a string", what)
		return "", false
	}
	return s, true
}

func (p *parser) seq(n *yamlx.Node, what string) []*yamlx.Node {
	if n == nil {
		return nil
	}
	if n.Kind != yamlx.Seq {
		p.errorf(n.Pos, "%s must be a sequence, got %s", what, n.Kind)
		return nil
	}
	return n.Items
}

// ParseWorkflow parses a workflow file.
func ParseWorkflow(n *yamlx.Node, name string) (*Workflow, error) {
	p := &parser{}
	wf := &Workflow{Name: name, Actions: map[string]*ActionDef{}}
	if !p.mapping(n, "workflow", "description", "actions", "steps") {
		return nil, p.errs
	}
	if d := n.Get("description"); d != nil {
		wf.Description, _ = p.str(d, "description")
	}
	if a := n.Get("actions"); a != nil && p.mapping(a, "actions", keys(a)...) {
		for _, pair := range a.Pairs {
			if !IsActionName(pair.Key) {
				p.errorf(pair.KeyPos, "invalid action name %q", pair.Key)
				continue
			}
			def := p.action(pair.Value, pair.Key, pair.KeyPos)
			def.Local = true
			wf.Actions[pair.Key] = def
		}
	}
	steps := n.Get("steps")
	if steps == nil {
		p.errorf(n.Pos, "workflow must have steps")
	}
	wf.Steps = p.steps(steps)
	return wf, p.errs.Err()
}

// ParseAction parses an action file.
func ParseAction(n *yamlx.Node, name string) (*ActionDef, error) {
	p := &parser{}
	def := p.action(n, name, n.Pos)
	return def, p.errs.Err()
}

func (p *parser) action(n *yamlx.Node, name string, pos yamlx.Pos) *ActionDef {
	def := &ActionDef{Name: name, Pos: pos}
	if !p.mapping(n, "action "+name, "description", "params", "steps", "outputs") {
		return def
	}
	if d := n.Get("description"); d != nil {
		def.Description, _ = p.str(d, "description")
	}
	if params := n.Get("params"); params != nil && p.mapping(params, "params", keys(params)...) {
		for _, pair := range params.Pairs {
			if !IsIdent(pair.Key) {
				p.errorf(pair.KeyPos, "invalid parameter name %q", pair.Key)
			}
			s, ok := p.str(pair.Value, "parameter type")
			if !ok {
				continue
			}
			t, ok := action.ParseType(s)
			if !ok {
				p.errorf(pair.Value.Pos, "unknown type %q (allowed: string, number, bool, object, array, any)", s)
				continue
			}
			def.Params = append(def.Params, &ParamDecl{Name: pair.Key, Pos: pair.KeyPos, Type: t})
		}
	}
	steps := n.Get("steps")
	if steps == nil {
		p.errorf(n.Pos, "action %s must have steps", name)
	}
	def.Steps = p.steps(steps)
	def.Outputs = p.bindings(n.Get("outputs"), "outputs")
	return def
}

func (p *parser) steps(n *yamlx.Node) []*Step {
	var steps []*Step
	for i, item := range p.seq(n, "steps") {
		if s := p.step(item, i); s != nil {
			steps = append(steps, s)
		}
	}
	return steps
}

func (p *parser) step(n *yamlx.Node, index int) *Step {
	if !p.mapping(n, "step", "id", "use", "inputs", "params", "foreach", "expects", "outputs") {
		return nil
	}
	s := &Step{Pos: n.Pos, Index: index}
	if id := n.Get("id"); id != nil {
		s.ID, _ = p.str(id, "id")
		s.IDPos = id.Pos
		if s.ID != "" && !IsIdent(s.ID) {
			p.errorf(id.Pos, "invalid step id %q (use letters, digits and underscores)", s.ID)
		}
	}
	if use := n.Get("use"); use != nil {
		s.Use, _ = p.str(use, "use")
		s.UsePos = use.Pos
	} else {
		p.errorf(n.Pos, "step must have use")
	}
	s.Inputs = p.bindings(n.Get("inputs"), "inputs")
	if params := n.Get("params"); params != nil {
		if params.Kind != yamlx.Map {
			p.errorf(params.Pos, "params must be a mapping, got %s", params.Kind)
		} else {
			v, err := tmpl.FromNode(params)
			p.add(err)
			if m, ok := v.(*tmpl.Map); ok {
				s.Params = m
			}
		}
	}
	if fe := n.Get("foreach"); fe != nil {
		v, err := tmpl.FromNode(fe)
		p.add(err)
		s.Foreach = v
	}
	for _, e := range p.seq(n.Get("expects"), "expects") {
		if ex := p.expect(e); ex != nil {
			s.Expects = append(s.Expects, ex)
		}
	}
	s.Outputs = p.bindings(n.Get("outputs"), "outputs")
	return s
}

func (p *parser) expect(n *yamlx.Node) *Expect {
	if !p.mapping(n, "expectation", "type", "status", "asserts") {
		return nil
	}
	ex := &Expect{Pos: n.Pos}
	if t := n.Get("type"); t != nil {
		ex.Type, _ = p.str(t, "type")
		ex.TypePos = t.Pos
	}
	if st := n.Get("status"); st != nil {
		if code, ok := st.Value.(int); ok {
			ex.Status = &code
		} else {
			p.errorf(st.Pos, "status must be an integer")
		}
		ex.StatusPos = st.Pos
	}
	for _, a := range p.seq(n.Get("asserts"), "asserts") {
		src, ok := p.str(a, "assert")
		if !ok {
			continue
		}
		if strings.Contains(src, "<<") {
			p.errorf(a.Pos, "asserts are expressions; write them without << >>")
			continue
		}
		e, err := tmpl.Compile(src, a.Pos)
		if err != nil {
			p.add(err)
			continue
		}
		ex.Asserts = append(ex.Asserts, e)
	}
	return ex
}

func (p *parser) bindings(n *yamlx.Node, what string) []*Binding {
	if n == nil || !p.mapping(n, what, keys(n)...) {
		return nil
	}
	var out []*Binding
	for _, pair := range n.Pairs {
		if !IsIdent(pair.Key) {
			p.errorf(pair.KeyPos, "invalid %s name %q (use letters, digits and underscores)", what, pair.Key)
			continue
		}
		v, err := tmpl.FromNode(pair.Value)
		if err != nil {
			p.add(err)
			continue
		}
		out = append(out, &Binding{Name: pair.Key, Pos: pair.KeyPos, Value: v})
	}
	return out
}

// ParseConfig parses config.yaml.
func ParseConfig(n *yamlx.Node) (*Config, error) {
	p := &parser{}
	cfg := &Config{HTTPTimeout: DefaultHTTPTimeout, Environments: map[string]*Environment{}}
	if !p.mapping(n, "config", "default", "http", "environments") {
		return nil, p.errs
	}
	if d := n.Get("default"); d != nil {
		cfg.Default, _ = p.str(d, "default")
	}
	if h := n.Get("http"); h != nil && p.mapping(h, "http", "timeout") {
		if t := h.Get("timeout"); t != nil {
			if s, ok := p.str(t, "timeout"); ok {
				d, err := time.ParseDuration(s)
				if err != nil || d <= 0 {
					p.errorf(t.Pos, "invalid timeout %q (example: 30s)", s)
				} else {
					cfg.HTTPTimeout = d
				}
			}
		}
	}
	if envs := n.Get("environments"); envs != nil && p.mapping(envs, "environments", keys(envs)...) {
		for _, pair := range envs.Pairs {
			env := &Environment{Name: pair.Key, Servers: map[string]*tmpl.Template{}}
			cfg.Environments[pair.Key] = env
			if !p.mapping(pair.Value, "environment "+pair.Key, "servers") {
				continue
			}
			servers := pair.Value.Get("servers")
			if servers == nil || !p.mapping(servers, "servers", keys(servers)...) {
				continue
			}
			for _, s := range servers.Pairs {
				src, ok := p.str(s.Value, "server URL")
				if !ok {
					continue
				}
				t, err := tmpl.Parse(src, s.Value.Pos)
				if err != nil {
					p.add(err)
					continue
				}
				for _, e := range t.Exprs() {
					for _, r := range e.Refs() {
						if r.Root != "env" {
							p.errorf(e.Pos, "only env can be referenced in config, got %q", r.Source)
						}
					}
				}
				env.Servers[s.Key] = t
			}
		}
	}
	if cfg.Default != "" && cfg.Environments[cfg.Default] == nil {
		p.errorf(n.Get("default").Pos, "default environment %q is not defined", cfg.Default)
	}
	return cfg, p.errs.Err()
}

func keys(n *yamlx.Node) []string {
	out := make([]string, len(n.Pairs))
	for i, p := range n.Pairs {
		out[i] = p.Key
	}
	return out
}
