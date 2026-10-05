// Package def defines the model of workflow, action and config definitions
// and parses them from YAML.
package def

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/internal/tmpl"
	"github.com/noi/dwbt/internal/yamlx"
)

// Workflow is a workflow definition.
type Workflow struct {
	// Name is the path of the file relative to the current directory.
	Name string
	// Path is the path of the file as loaded.
	Path        string
	Description string
	// Actions are the actions defined in the workflow file.
	Actions map[string]*ActionDef
	Steps   []*Step
}

// ActionDef is a user-defined action.
type ActionDef struct {
	Name        string
	Pos         yamlx.Pos
	Description string
	Params      []*ParamDecl
	Steps       []*Step
	Outputs     []*Binding
	// Local reports whether the action is defined in a workflow file.
	Local bool
}

// ParamDecl declares a parameter of a user-defined action.
type ParamDecl struct {
	Name string
	Pos  yamlx.Pos
	Type action.Type
}

// Step is a step of a workflow or of a user-defined action.
type Step struct {
	Pos yamlx.Pos
	// Index is the position of the step in its list, starting at 0.
	Index   int
	ID      string
	IDPos   yamlx.Pos
	Use     string
	UsePos  yamlx.Pos
	Inputs  []*Binding
	Params  *tmpl.Map
	Foreach tmpl.Value
	Expects []*Expect
	Outputs []*Binding
}

// Label returns a human-readable name of the step.
func (s *Step) Label() string {
	if s.ID != "" {
		return s.ID + " (" + s.Use + ")"
	}
	return "#" + strconv.Itoa(s.Index+1) + " (" + s.Use + ")"
}

// Binding is a named value, used for inputs and outputs.
type Binding struct {
	Name  string
	Pos   yamlx.Pos
	Value tmpl.Value
}

// Expect is an expectation of a step.
type Expect struct {
	Pos yamlx.Pos
	// Type is the explicitly specified type, or "" if omitted.
	Type      string
	TypePos   yamlx.Pos
	Status    *int
	StatusPos yamlx.Pos
	Asserts   []*tmpl.Expr
}

// Config is the content of config.yaml.
type Config struct {
	Default      string
	HTTPTimeout  time.Duration
	Environments map[string]*Environment
}

// Environment is an environment profile.
type Environment struct {
	Name    string
	Servers map[string]*tmpl.Template
}

// DefaultHTTPTimeout is used when config.yaml does not set http.timeout.
const DefaultHTTPTimeout = 30 * time.Second

// Errors is a list of definition errors.
type Errors []error

func (e Errors) Error() string {
	switch len(e) {
	case 0:
		return "no errors"
	case 1:
		return e[0].Error()
	}
	return fmt.Sprintf("%v (and %d more errors)", e[0], len(e)-1)
}

// Err returns e as an error, or nil if it is empty.
func (e Errors) Err() error {
	if len(e) == 0 {
		return nil
	}
	return e
}

var (
	identRe      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	actionNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+(/[A-Za-z0-9_-]+)*$`)
)

// IsIdent reports whether s can be used as a step id, parameter, input or
// output name. Such names must be accessible with the dot syntax in
// expressions.
func IsIdent(s string) bool { return identRe.MatchString(s) }

// IsActionName reports whether s is a valid action name, like "user/create".
func IsActionName(s string) bool { return actionNameRe.MatchString(s) }
