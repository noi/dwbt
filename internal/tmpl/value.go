package tmpl

import (
	"github.com/noi/dwbt/internal/yamlx"
)

// Value is a YAML value whose strings are templates.
type Value interface {
	Eval(env Env) (any, error)
	// Exprs returns all expressions in the value.
	Exprs() []*Expr
	Position() yamlx.Pos
}

// FromNode builds a Value from a YAML node. Every string scalar is parsed as
// a template; map keys are used literally.
func FromNode(n *yamlx.Node) (Value, error) {
	switch n.Kind {
	case yamlx.Map:
		m := &Map{Pos: n.Pos}
		for _, p := range n.Pairs {
			v, err := FromNode(p.Value)
			if err != nil {
				return nil, err
			}
			m.Keys = append(m.Keys, p.Key)
			m.KeyPos = append(m.KeyPos, p.KeyPos)
			m.Values = append(m.Values, v)
		}
		return m, nil
	case yamlx.Seq:
		l := &List{Pos: n.Pos}
		for _, it := range n.Items {
			v, err := FromNode(it)
			if err != nil {
				return nil, err
			}
			l.Items = append(l.Items, v)
		}
		return l, nil
	default:
		if s, ok := n.Value.(string); ok {
			t, err := Parse(s, n.Pos)
			if err != nil {
				return nil, err
			}
			return &String{t}, nil
		}
		return &Literal{Pos: n.Pos, V: n.Value}, nil
	}
}

// Literal is a non-string scalar.
type Literal struct {
	Pos yamlx.Pos
	V   any
}

func (l *Literal) Eval(Env) (any, error) { return l.V, nil }
func (l *Literal) Exprs() []*Expr        { return nil }
func (l *Literal) Position() yamlx.Pos   { return l.Pos }

// String is a string scalar.
type String struct {
	*Template
}

func (s *String) Position() yamlx.Pos { return s.Pos }

// Map is a mapping.
type Map struct {
	Pos    yamlx.Pos
	Keys   []string
	KeyPos []yamlx.Pos
	Values []Value
}

func (m *Map) Eval(env Env) (any, error) {
	out := make(map[string]any, len(m.Keys))
	for i, k := range m.Keys {
		v, err := m.Values[i].Eval(env)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

func (m *Map) Exprs() []*Expr {
	var out []*Expr
	for _, v := range m.Values {
		out = append(out, v.Exprs()...)
	}
	return out
}

func (m *Map) Position() yamlx.Pos { return m.Pos }

// List is a sequence.
type List struct {
	Pos   yamlx.Pos
	Items []Value
}

func (l *List) Eval(env Env) (any, error) {
	out := make([]any, len(l.Items))
	for i, it := range l.Items {
		v, err := it.Eval(env)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func (l *List) Exprs() []*Expr {
	var out []*Expr
	for _, it := range l.Items {
		out = append(out, it.Exprs()...)
	}
	return out
}

func (l *List) Position() yamlx.Pos { return l.Pos }
