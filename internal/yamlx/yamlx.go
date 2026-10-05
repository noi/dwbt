// Package yamlx parses YAML into a simple node tree that keeps source
// positions, so that later stages can report errors with line numbers.
package yamlx

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Pos is a position in a source file.
type Pos struct {
	File string
	Line int
	Col  int
}

func (p Pos) String() string {
	if p.Line == 0 {
		return p.File
	}
	return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col)
}

// Error is an error tied to a source position.
type Error struct {
	Pos Pos
	Msg string
}

func (e *Error) Error() string { return e.Pos.String() + ": " + e.Msg }

// Errorf returns an *Error at pos.
func Errorf(pos Pos, format string, args ...any) *Error {
	return &Error{Pos: pos, Msg: fmt.Sprintf(format, args...)}
}

// Kind is the kind of a Node.
type Kind int

const (
	Scalar Kind = iota
	Map
	Seq
)

func (k Kind) String() string {
	switch k {
	case Map:
		return "mapping"
	case Seq:
		return "sequence"
	default:
		return "scalar"
	}
}

// Node is a YAML node.
type Node struct {
	Kind  Kind
	Pos   Pos
	Pairs []Pair  // for Map, in source order
	Items []*Node // for Seq
	Value any     // for Scalar: string, int, float64, bool or nil
}

// Pair is a key-value entry of a mapping.
type Pair struct {
	Key    string
	KeyPos Pos
	Value  *Node
}

// Get returns the value for key, or nil if the mapping has no such key.
func (n *Node) Get(key string) *Node {
	for _, p := range n.Pairs {
		if p.Key == key {
			return p.Value
		}
	}
	return nil
}

// Interface converts the node into plain Go values
// (map[string]any, []any and scalars).
func (n *Node) Interface() any {
	switch n.Kind {
	case Map:
		m := make(map[string]any, len(n.Pairs))
		for _, p := range n.Pairs {
			m[p.Key] = p.Value.Interface()
		}
		return m
	case Seq:
		s := make([]any, len(n.Items))
		for i, it := range n.Items {
			s[i] = it.Interface()
		}
		return s
	default:
		return n.Value
	}
}

// ParseFile reads and parses the YAML file at path. name is used as the file
// name in positions. An empty document yields an empty mapping.
func ParseFile(path, name string) (*Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data, name)
}

// Parse parses YAML data. file is used as the file name in positions.
func Parse(data []byte, file string) (*Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, &Error{Pos: Pos{File: file}, Msg: strings.TrimPrefix(err.Error(), "yaml: ")}
	}
	if len(doc.Content) == 0 {
		return &Node{Kind: Map, Pos: Pos{File: file, Line: 1, Col: 1}}, nil
	}
	c := converter{file: file}
	return c.convert(doc.Content[0], 0)
}

type converter struct {
	file string
}

const maxDepth = 1000

func (c *converter) pos(n *yaml.Node) Pos {
	return Pos{File: c.file, Line: n.Line, Col: n.Column}
}

func (c *converter) convert(n *yaml.Node, depth int) (*Node, error) {
	if depth > maxDepth {
		return nil, Errorf(c.pos(n), "document is nested too deeply")
	}
	switch n.Kind {
	case yaml.AliasNode:
		return c.convert(n.Alias, depth+1)
	case yaml.MappingNode:
		out := &Node{Kind: Map, Pos: c.pos(n)}
		seen := map[string]bool{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.ShortTag() == "!!merge" {
				return nil, Errorf(c.pos(k), "merge keys (<<) are not supported")
			}
			if k.Kind != yaml.ScalarNode {
				return nil, Errorf(c.pos(k), "mapping keys must be scalars")
			}
			if seen[k.Value] {
				return nil, Errorf(c.pos(k), "duplicate key %q", k.Value)
			}
			seen[k.Value] = true
			val, err := c.convert(v, depth+1)
			if err != nil {
				return nil, err
			}
			out.Pairs = append(out.Pairs, Pair{Key: k.Value, KeyPos: c.pos(k), Value: val})
		}
		return out, nil
	case yaml.SequenceNode:
		out := &Node{Kind: Seq, Pos: c.pos(n)}
		for _, it := range n.Content {
			val, err := c.convert(it, depth+1)
			if err != nil {
				return nil, err
			}
			out.Items = append(out.Items, val)
		}
		return out, nil
	case yaml.ScalarNode:
		v, err := scalar(n)
		if err != nil {
			return nil, Errorf(c.pos(n), "%v", err)
		}
		return &Node{Kind: Scalar, Pos: c.pos(n), Value: v}, nil
	default:
		return nil, Errorf(c.pos(n), "unsupported YAML node")
	}
}

func scalar(n *yaml.Node) (any, error) {
	switch n.ShortTag() {
	case "!!null":
		return nil, nil
	case "!!bool":
		var b bool
		err := n.Decode(&b)
		return b, err
	case "!!int":
		var i int
		if err := n.Decode(&i); err != nil {
			return nil, errors.New("integer out of range: " + n.Value)
		}
		return i, nil
	case "!!float":
		var f float64
		err := n.Decode(&f)
		return f, err
	default:
		return n.Value, nil
	}
}
