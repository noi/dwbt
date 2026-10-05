package tmpl

import (
	"strings"

	"github.com/noi/dwbt/internal/value"
	"github.com/noi/dwbt/internal/yamlx"
)

const (
	open  = "<<"
	close = ">>"
)

// Template is a string that may contain << >> expressions.
type Template struct {
	Pos   yamlx.Pos
	parts []part
}

type part struct {
	lit  string
	expr *Expr
}

// Parse parses a template string found at pos.
func Parse(s string, pos yamlx.Pos) (*Template, error) {
	t := &Template{Pos: pos}
	for {
		i := strings.Index(s, open)
		if i < 0 {
			break
		}
		if i > 0 {
			t.parts = append(t.parts, part{lit: s[:i]})
		}
		rest := s[i+len(open):]
		j, err := closeIndex(rest)
		if err != nil {
			return nil, yamlx.Errorf(pos, "%v in %q", err, s)
		}
		src := strings.TrimSpace(rest[:j])
		if src == "" {
			return nil, yamlx.Errorf(pos, "empty expression in %q", s)
		}
		e, err := Compile(src, pos)
		if err != nil {
			return nil, err
		}
		t.parts = append(t.parts, part{expr: e})
		s = rest[j+len(close):]
	}
	if s != "" || len(t.parts) == 0 {
		t.parts = append(t.parts, part{lit: s})
	}
	return t, nil
}

// closeIndex returns the index of the >> that closes an expression, skipping
// string literals inside the expression.
func closeIndex(s string) (int, error) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"', '\'', '`':
			end := i + 1
			for end < len(s) && s[end] != c {
				if s[end] == '\\' && c != '`' {
					end++
				}
				end++
			}
			if end >= len(s) {
				return 0, errUnterminatedString
			}
			i = end
		case '>':
			if strings.HasPrefix(s[i:], close) {
				return i, nil
			}
		}
	}
	return 0, errUnclosed
}

type parseError string

func (e parseError) Error() string { return string(e) }

const (
	errUnclosed           = parseError("unclosed <<")
	errUnterminatedString = parseError("unterminated string literal in expression")
)

// IsLiteral reports whether the template contains no expressions.
func (t *Template) IsLiteral() bool {
	return len(t.parts) == 1 && t.parts[0].expr == nil
}

// Exprs returns the expressions in the template.
func (t *Template) Exprs() []*Expr {
	var out []*Expr
	for _, p := range t.parts {
		if p.expr != nil {
			out = append(out, p.expr)
		}
	}
	return out
}

// Eval evaluates the template. A template consisting of a single expression
// yields the expression's value as is; otherwise the parts are converted to
// strings and concatenated.
func (t *Template) Eval(env Env) (any, error) {
	if len(t.parts) == 1 {
		if e := t.parts[0].expr; e != nil {
			return e.Eval(env)
		}
		return t.parts[0].lit, nil
	}
	var b strings.Builder
	for _, p := range t.parts {
		if p.expr == nil {
			b.WriteString(p.lit)
			continue
		}
		v, err := p.expr.Eval(env)
		if err != nil {
			return nil, err
		}
		b.WriteString(value.String(v))
	}
	return b.String(), nil
}
