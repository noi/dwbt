// Package tmpl implements expressions and << >> templates on top of expr.
package tmpl

import (
	"crypto/rand"
	"fmt"
	"slices"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/vm"

	"github.com/noi/dwbt/internal/value"
	"github.com/noi/dwbt/internal/yamlx"
)

// Env holds the variables visible to an expression.
type Env map[string]any

// With returns a copy of e with the given variable set.
func (e Env) With(name string, v any) Env {
	out := make(Env, len(e)+1)
	for k, val := range e {
		out[k] = val
	}
	out[name] = v
	return out
}

var options = []expr.Option{
	expr.Function("uuid", func(params ...any) (any, error) {
		return newUUID(), nil
	}, new(func() string)),
}

// Expr is a compiled expression.
type Expr struct {
	Source string
	Pos    yamlx.Pos
	prog   *vm.Program
	refs   []Ref
}

// Ref is a reference to a variable found in an expression.
type Ref struct {
	// Root is the variable name, like "inputs".
	Root string
	// Path holds the constant property names that follow Root, up to the
	// first computed one. For "outputs.prepare.users[0].id" it is
	// ["prepare", "users"].
	Path []string
	// Source is the source text of the whole member chain.
	Source string
}

// Compile compiles an expression found at pos.
func Compile(src string, pos yamlx.Pos) (*Expr, error) {
	tree, err := parser.Parse(src)
	if err != nil {
		return nil, yamlx.Errorf(pos, "invalid expression %q: %v", src, firstLine(err))
	}
	prog, err := expr.Compile(src, options...)
	if err != nil {
		return nil, yamlx.Errorf(pos, "invalid expression %q: %v", src, firstLine(err))
	}
	a := analyzer{locals: map[string]bool{}}
	a.visit(tree.Node)
	return &Expr{Source: src, Pos: pos, prog: prog, refs: a.refs}, nil
}

// Refs returns the variable references in the expression.
func (e *Expr) Refs() []Ref { return e.refs }

// Eval evaluates the expression.
func (e *Expr) Eval(env Env) (any, error) {
	v, err := expr.Run(e.prog, map[string]any(env))
	if err != nil {
		return nil, yamlx.Errorf(e.Pos, "evaluating %q: %v", e.Source, firstLine(err))
	}
	return v, nil
}

// Explain evaluates each variable reference of the expression and returns
// lines like "inputs.users[0].id = 3", to show why an assertion failed.
func (e *Expr) Explain(env Env) []string {
	var lines []string
	var seen []string
	for _, r := range e.refs {
		if slices.Contains(seen, r.Source) {
			continue
		}
		seen = append(seen, r.Source)
		v, err := expr.Eval(r.Source, map[string]any(env))
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s = <error: %v>", r.Source, firstLine(err)))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s = %s", r.Source, value.Format(v)))
	}
	return lines
}

func firstLine(err error) string {
	s := err.Error()
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}

type analyzer struct {
	refs   []Ref
	locals map[string]bool
}

func (a *analyzer) visit(n ast.Node) {
	switch n := n.(type) {
	case nil:
	case *ast.IdentifierNode:
		if !a.locals[n.Value] {
			a.refs = append(a.refs, Ref{Root: n.Value, Source: n.String()})
		}
	case *ast.MemberNode:
		a.member(n)
	case *ast.ChainNode:
		a.visit(n.Node)
	case *ast.CallNode:
		if _, ok := n.Callee.(*ast.IdentifierNode); !ok {
			a.visit(n.Callee)
		}
		for _, arg := range n.Arguments {
			a.visit(arg)
		}
	case *ast.BuiltinNode:
		for _, arg := range n.Arguments {
			a.visit(arg)
		}
	case *ast.PredicateNode:
		a.visit(n.Node)
	case *ast.UnaryNode:
		a.visit(n.Node)
	case *ast.BinaryNode:
		a.visit(n.Left)
		a.visit(n.Right)
	case *ast.ConditionalNode:
		a.visit(n.Cond)
		a.visit(n.Exp1)
		a.visit(n.Exp2)
	case *ast.SliceNode:
		a.visit(n.Node)
		a.visit(n.From)
		a.visit(n.To)
	case *ast.VariableDeclaratorNode:
		a.visit(n.Value)
		a.locals[n.Name] = true
		a.visit(n.Expr)
	case *ast.SequenceNode:
		for _, c := range n.Nodes {
			a.visit(c)
		}
	case *ast.ArrayNode:
		for _, c := range n.Nodes {
			a.visit(c)
		}
	case *ast.MapNode:
		for _, c := range n.Pairs {
			a.visit(c)
		}
	case *ast.PairNode:
		a.visit(n.Key)
		a.visit(n.Value)
	}
}

// member records the member chain rooted at an identifier, and visits the
// expressions nested in it (computed properties, non-identifier roots).
func (a *analyzer) member(n *ast.MemberNode) {
	var props []ast.Node
	var root ast.Node = n
	for {
		m, ok := root.(*ast.MemberNode)
		if !ok {
			break
		}
		props = append(props, m.Property)
		root = m.Node
	}
	slices.Reverse(props)

	id, ok := root.(*ast.IdentifierNode)
	if ok && !a.locals[id.Value] {
		r := Ref{Root: id.Value, Source: n.String()}
		for _, p := range props {
			s, ok := p.(*ast.StringNode)
			if !ok {
				break
			}
			r.Path = append(r.Path, s.Value)
		}
		a.refs = append(a.refs, r)
	} else if !ok {
		a.visit(root)
	}
	for _, p := range props {
		if _, ok := p.(*ast.StringNode); !ok {
			a.visit(p)
		}
	}
}

func newUUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
