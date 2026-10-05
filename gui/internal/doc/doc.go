// Package doc converts workflow and action files to and from documents that
// the editor can manipulate.
//
// A document keeps the structure of a definition, while the values of
// inputs, params, foreach and outputs are kept as YAML source text so that
// their style and inner comments survive a round trip. Comments outside of
// values and the formatting of the structure itself are not preserved, nor
// is the type of an expectation: it can only be the default type of the
// action, so stating it has no effect.
package doc

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Workflow is a workflow file.
type Workflow struct {
	Description string `json:"description"`
	// Actions are the actions local to the workflow file.
	Actions []*Action `json:"actions"`
	// Setup is nil if the workflow has no setup.
	Setup    *Setup     `json:"setup"`
	Inputs   []*Binding `json:"inputs"`
	Steps    []*Step    `json:"steps"`
	Teardown []*Step    `json:"teardown"`
}

// Setup is the setup of a workflow.
type Setup struct {
	Steps   []*Step    `json:"steps"`
	Outputs []*Binding `json:"outputs"`
}

// Action is a user-defined action.
type Action struct {
	// Name is the key of a local action. It is empty for an action file,
	// whose name derives from its path.
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Params      []*Param   `json:"params"`
	Steps       []*Step    `json:"steps"`
	Outputs     []*Binding `json:"outputs"`
}

// Param declares a parameter of an action.
type Param struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Step is a step of a workflow or of an action.
type Step struct {
	ID     string     `json:"id"`
	Use    string     `json:"use"`
	Inputs []*Binding `json:"inputs"`
	// Foreach is the YAML source of the foreach value, or "" if omitted.
	Foreach string     `json:"foreach"`
	Params  []*Binding `json:"params"`
	Expects []*Expect  `json:"expects"`
	Outputs []*Binding `json:"outputs"`
}

// Binding is a named value. Value is its YAML source; an empty Value is
// encoded as null.
type Binding struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Expect is an expectation of a step.
type Expect struct {
	Status  *int     `json:"status"`
	Asserts []string `json:"asserts"`
}

// ParseWorkflow parses the content of a workflow file.
func ParseWorkflow(data []byte) (*Workflow, error) {
	n, err := root(data)
	if err != nil {
		return nil, err
	}
	wf := &Workflow{Actions: []*Action{}, Inputs: []*Binding{}, Steps: []*Step{}, Teardown: []*Step{}}
	err = fields(n, "workflow", map[string]func(*yaml.Node) error{
		"description": func(v *yaml.Node) (err error) {
			wf.Description, err = str(v, "description")
			return err
		},
		"actions": func(v *yaml.Node) error {
			if v.Kind != yaml.MappingNode {
				return errorf(v, "actions must be a mapping")
			}
			for i := 0; i+1 < len(v.Content); i += 2 {
				a, err := parseAction(v.Content[i+1], v.Content[i].Value)
				if err != nil {
					return err
				}
				a.Name = v.Content[i].Value
				wf.Actions = append(wf.Actions, a)
			}
			return nil
		},
		"setup": func(v *yaml.Node) error {
			wf.Setup = &Setup{Steps: []*Step{}, Outputs: []*Binding{}}
			return fields(v, "setup", map[string]func(*yaml.Node) error{
				"steps": func(v *yaml.Node) (err error) {
					wf.Setup.Steps, err = steps(v)
					return err
				},
				"outputs": func(v *yaml.Node) (err error) {
					wf.Setup.Outputs, err = bindings(v, "outputs")
					return err
				},
			})
		},
		"inputs": func(v *yaml.Node) (err error) {
			wf.Inputs, err = bindings(v, "inputs")
			return err
		},
		"steps": func(v *yaml.Node) (err error) {
			wf.Steps, err = steps(v)
			return err
		},
		"teardown": func(v *yaml.Node) (err error) {
			wf.Teardown, err = steps(v)
			return err
		},
	})
	if err != nil {
		return nil, err
	}
	return wf, nil
}

// ParseAction parses the content of an action file.
func ParseAction(data []byte) (*Action, error) {
	n, err := root(data)
	if err != nil {
		return nil, err
	}
	return parseAction(n, "action")
}

func root(data []byte) (*yaml.Node, error) {
	var d yaml.Node
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	if len(d.Content) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode}, nil
	}
	return d.Content[0], nil
}

func parseAction(n *yaml.Node, what string) (*Action, error) {
	a := &Action{Params: []*Param{}, Steps: []*Step{}, Outputs: []*Binding{}}
	err := fields(n, what, map[string]func(*yaml.Node) error{
		"description": func(v *yaml.Node) (err error) {
			a.Description, err = str(v, "description")
			return err
		},
		"params": func(v *yaml.Node) error {
			if v.Kind != yaml.MappingNode {
				return errorf(v, "params must be a mapping")
			}
			for i := 0; i+1 < len(v.Content); i += 2 {
				t, err := str(v.Content[i+1], "parameter type")
				if err != nil {
					return err
				}
				a.Params = append(a.Params, &Param{Name: v.Content[i].Value, Type: t})
			}
			return nil
		},
		"steps": func(v *yaml.Node) (err error) {
			a.Steps, err = steps(v)
			return err
		},
		"outputs": func(v *yaml.Node) (err error) {
			a.Outputs, err = bindings(v, "outputs")
			return err
		},
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

func steps(n *yaml.Node) ([]*Step, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, errorf(n, "steps must be a sequence")
	}
	out := []*Step{}
	for _, item := range n.Content {
		s := &Step{Inputs: []*Binding{}, Params: []*Binding{}, Expects: []*Expect{}, Outputs: []*Binding{}}
		err := fields(item, "step", map[string]func(*yaml.Node) error{
			"id": func(v *yaml.Node) (err error) {
				s.ID, err = str(v, "id")
				return err
			},
			"use": func(v *yaml.Node) (err error) {
				s.Use, err = str(v, "use")
				return err
			},
			"inputs": func(v *yaml.Node) (err error) {
				s.Inputs, err = bindings(v, "inputs")
				return err
			},
			"foreach": func(v *yaml.Node) (err error) {
				s.Foreach, err = Source(v)
				return err
			},
			"params": func(v *yaml.Node) (err error) {
				s.Params, err = bindings(v, "params")
				return err
			},
			"expects": func(v *yaml.Node) (err error) {
				s.Expects, err = expects(v)
				return err
			},
			"outputs": func(v *yaml.Node) (err error) {
				s.Outputs, err = bindings(v, "outputs")
				return err
			},
		})
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func expects(n *yaml.Node) ([]*Expect, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, errorf(n, "expects must be a sequence")
	}
	out := []*Expect{}
	for _, item := range n.Content {
		ex := &Expect{Asserts: []string{}}
		err := fields(item, "expectation", map[string]func(*yaml.Node) error{
			"type": func(v *yaml.Node) error {
				_, err := str(v, "type")
				return err
			},
			"status": func(v *yaml.Node) error {
				code, err := strconv.Atoi(v.Value)
				if v.Kind != yaml.ScalarNode || err != nil {
					return errorf(v, "status must be an integer")
				}
				ex.Status = &code
				return nil
			},
			"asserts": func(v *yaml.Node) error {
				if v.Kind != yaml.SequenceNode {
					return errorf(v, "asserts must be a sequence")
				}
				for _, a := range v.Content {
					s, err := str(a, "assert")
					if err != nil {
						return err
					}
					ex.Asserts = append(ex.Asserts, s)
				}
				return nil
			},
		})
		if err != nil {
			return nil, err
		}
		out = append(out, ex)
	}
	return out, nil
}

func bindings(n *yaml.Node, what string) ([]*Binding, error) {
	if n.Kind != yaml.MappingNode {
		return nil, errorf(n, "%s must be a mapping", what)
	}
	out := []*Binding{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		src, err := Source(n.Content[i+1])
		if err != nil {
			return nil, err
		}
		out = append(out, &Binding{Name: n.Content[i].Value, Value: src})
	}
	return out, nil
}

// fields calls the handler of each key of the mapping n. Unknown keys are
// rejected since the document could not keep them.
func fields(n *yaml.Node, what string, handlers map[string]func(*yaml.Node) error) error {
	if n.Kind != yaml.MappingNode {
		return errorf(n, "%s must be a mapping", what)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		h, ok := handlers[k.Value]
		if !ok {
			return errorf(k, "unknown key %q in %s", k.Value, what)
		}
		if err := h(n.Content[i+1]); err != nil {
			return err
		}
	}
	return nil
}

func str(n *yaml.Node, what string) (string, error) {
	if n.Kind != yaml.ScalarNode || n.Tag == "!!null" {
		return "", errorf(n, "%s must be a string", what)
	}
	return n.Value, nil
}

func errorf(n *yaml.Node, format string, args ...any) error {
	return fmt.Errorf("line %d: %s", n.Line, fmt.Sprintf(format, args...))
}

// Source returns the YAML source of a value.
func Source(n *yaml.Node) (string, error) {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!null" && n.Value == "" {
		return "", nil
	}
	b, err := encode(n)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(b), "\n"), nil
}

// Value parses the YAML source of a value. Empty source is null.
func Value(src string) (*yaml.Node, error) {
	var d yaml.Node
	if err := yaml.Unmarshal([]byte(src), &d); err != nil {
		return nil, err
	}
	if len(d.Content) == 0 {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null"}, nil
	}
	return d.Content[0], nil
}

// EncodeWorkflow encodes wf as the content of a workflow file.
func EncodeWorkflow(wf *Workflow) ([]byte, error) {
	m := &mapping{}
	m.str("description", wf.Description)
	if len(wf.Actions) > 0 {
		actions := &mapping{}
		for _, a := range wf.Actions {
			n, err := actionNode(a)
			if err != nil {
				return nil, fmt.Errorf("action %s: %w", a.Name, err)
			}
			actions.add(a.Name, n)
		}
		m.add("actions", actions.node())
	}
	if wf.Setup != nil {
		setup := &mapping{}
		s, err := stepsNode(wf.Setup.Steps)
		if err != nil {
			return nil, fmt.Errorf("setup: %w", err)
		}
		setup.add("steps", s)
		if err := setup.bindings("outputs", wf.Setup.Outputs); err != nil {
			return nil, fmt.Errorf("setup: %w", err)
		}
		m.add("setup", setup.node())
	}
	if err := m.bindings("inputs", wf.Inputs); err != nil {
		return nil, err
	}
	s, err := stepsNode(wf.Steps)
	if err != nil {
		return nil, err
	}
	m.add("steps", s)
	if len(wf.Teardown) > 0 {
		t, err := stepsNode(wf.Teardown)
		if err != nil {
			return nil, fmt.Errorf("teardown: %w", err)
		}
		m.add("teardown", t)
	}
	b, err := encode(m.node())
	if err != nil {
		return nil, err
	}
	return separateSteps(b), nil
}

// EncodeAction encodes a as the content of an action file. a.Name is
// ignored.
func EncodeAction(a *Action) ([]byte, error) {
	n, err := actionNode(a)
	if err != nil {
		return nil, err
	}
	b, err := encode(n)
	if err != nil {
		return nil, err
	}
	return separateSteps(b), nil
}

func actionNode(a *Action) (*yaml.Node, error) {
	m := &mapping{}
	m.str("description", a.Description)
	if len(a.Params) > 0 {
		params := &mapping{}
		for _, p := range a.Params {
			params.str(p.Name, p.Type)
		}
		m.add("params", params.node())
	}
	s, err := stepsNode(a.Steps)
	if err != nil {
		return nil, err
	}
	m.add("steps", s)
	if err := m.bindings("outputs", a.Outputs); err != nil {
		return nil, err
	}
	return m.node(), nil
}

func stepsNode(steps []*Step) (*yaml.Node, error) {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for i, s := range steps {
		n, err := stepNode(s)
		if err != nil {
			return nil, fmt.Errorf("step %d: %w", i+1, err)
		}
		seq.Content = append(seq.Content, n)
	}
	return seq, nil
}

func stepNode(s *Step) (*yaml.Node, error) {
	m := &mapping{}
	m.str("id", s.ID)
	m.add("use", scalar(s.Use))
	if err := m.bindings("inputs", s.Inputs); err != nil {
		return nil, err
	}
	if strings.TrimSpace(s.Foreach) != "" {
		v, err := Value(s.Foreach)
		if err != nil {
			return nil, fmt.Errorf("foreach: %w", err)
		}
		m.add("foreach", v)
	}
	if err := m.bindings("params", s.Params); err != nil {
		return nil, err
	}
	if len(s.Expects) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, ex := range s.Expects {
			e := &mapping{}
			if ex.Status != nil {
				e.add("status", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(*ex.Status)})
			}
			if len(ex.Asserts) > 0 {
				asserts := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
				for _, a := range ex.Asserts {
					asserts.Content = append(asserts.Content, scalar(a))
				}
				e.add("asserts", asserts)
			}
			seq.Content = append(seq.Content, e.node())
		}
		m.add("expects", seq)
	}
	if err := m.bindings("outputs", s.Outputs); err != nil {
		return nil, err
	}
	return m.node(), nil
}

// mapping builds a mapping node keeping the order of the keys.
type mapping struct {
	content []*yaml.Node
}

func (m *mapping) add(key string, v *yaml.Node) {
	m.content = append(m.content, scalar(key), v)
}

// str adds a string value unless it is empty.
func (m *mapping) str(key, v string) {
	if v != "" {
		m.add(key, scalar(v))
	}
}

func (m *mapping) bindings(key string, bs []*Binding) error {
	if len(bs) == 0 {
		return nil
	}
	sub := &mapping{}
	for _, b := range bs {
		v, err := Value(b.Value)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", key, b.Name, err)
		}
		sub.add(b.Name, v)
	}
	m.add(key, sub.node())
	return nil
}

func (m *mapping) node() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: m.content}
}

func scalar(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

func encode(n *yaml.Node) ([]byte, error) {
	var b bytes.Buffer
	e := yaml.NewEncoder(&b)
	e.SetIndent(2)
	if err := e.Encode(n); err != nil {
		return nil, err
	}
	if err := e.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// separateSteps inserts a blank line between the top-level steps, the
// steps of the setup and those of the teardown, as they are usually written
// by hand.
func separateSteps(b []byte) []byte {
	lines := strings.SplitAfter(string(b), "\n")
	var out []string
	// item is the prefix of the steps of the current list, if any.
	item := ""
	inSetup := false
	for _, l := range lines {
		switch {
		case !strings.HasPrefix(l, " "):
			inSetup = l == "setup:\n"
			item = ""
			if l == "steps:\n" || l == "teardown:\n" {
				item = "  - "
			}
		case inSetup && !strings.HasPrefix(l, "   "):
			item = ""
			if l == "  steps:\n" {
				item = "    - "
			}
		case item != "" && strings.HasPrefix(l, item) && !strings.HasSuffix(out[len(out)-1], "steps:\n") && !strings.HasSuffix(out[len(out)-1], "teardown:\n"):
			out = append(out, "\n")
		}
		out = append(out, l)
	}
	return []byte(strings.Join(out, ""))
}
