// Package action defines the interface implemented by actions written in Go,
// such as the built-in http action.
package action

import (
	"context"
	"time"

	"github.com/noi/dwbt/internal/value"
)

// Type is the type of an action parameter.
type Type string

const (
	String Type = "string"
	Number Type = "number"
	Bool   Type = "bool"
	Object Type = "object"
	Array  Type = "array"
	Any    Type = "any"
)

// ParseType parses a type name used in workflow definitions.
func ParseType(s string) (Type, bool) {
	switch t := Type(s); t {
	case String, Number, Bool, Object, Array, Any:
		return t, true
	}
	return "", false
}

// Accepts reports whether v is a value of type t.
func (t Type) Accepts(v any) bool {
	switch t {
	case Any:
		return true
	case Number:
		return value.IsNumber(v)
	default:
		return value.TypeName(v) == string(t)
	}
}

// Param describes a parameter of an action.
type Param struct {
	Type     Type
	Required bool
}

// ParamSpec maps parameter names to their descriptions.
type ParamSpec map[string]Param

// Runtime gives actions access to the execution environment.
type Runtime interface {
	// Server returns the base URL of the server with the given id, which may
	// be a unix: URL of a Unix domain socket; see httpaction.Action.
	Server(id string) (string, error)
	// HTTPTimeout returns the timeout for a single HTTP request.
	HTTPTimeout() time.Duration
}

// Action is an action implemented in Go.
//
// Params and Outputs are used to validate workflows before they run. Run
// receives evaluated parameters whose types have already been checked against
// Params, and returns the outputs, which must contain exactly the keys listed
// by Outputs.
//
// Run may be called concurrently when workflows run in parallel.
type Action interface {
	Params() ParamSpec
	Outputs() []string
	Run(ctx context.Context, rt Runtime, params map[string]any) (map[string]any, error)
}

// Kinded is implemented by actions whose results support a dedicated
// expectation type, such as "http". Steps using such an action default to
// that type when an expectation omits it.
type Kinded interface {
	Kind() string
}

// KindOf returns the expectation type of a, or "" if it has none.
func KindOf(a Action) string {
	if k, ok := a.(Kinded); ok {
		return k.Kind()
	}
	return ""
}
