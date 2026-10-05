// Package dwbt runs workflow-based tests defined in YAML.
//
// The dwbt command can be extended with actions written in Go by building
// a custom binary:
//
//	func main() {
//		dwbt.New().Action("db/seed", seedAction{}).Main()
//	}
package dwbt

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/signal"
	"slices"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/action/httpaction"
	"github.com/noi/dwbt/internal/cli"
	"github.com/noi/dwbt/internal/def"
)

// Command is the dwbt command. Its methods return a new Command and leave
// the receiver unchanged, so a Command can be shared and extended safely.
type Command struct {
	actions map[string]action.Action
	errs    []error
}

// New returns the dwbt command with the built-in actions.
func New() Command {
	return Command{actions: map[string]action.Action{"http": httpaction.New()}}
}

// Action registers an action implemented in Go under name, such as
// "db/seed". It cannot replace a built-in action or an action registered
// before.
func (c Command) Action(name string, a action.Action) Command {
	switch {
	case !def.IsActionName(name):
		return c.withError(fmt.Errorf("invalid action name %q", name))
	case c.actions[name] != nil:
		return c.withError(fmt.Errorf("action %q is already registered", name))
	}
	actions := maps.Clone(c.actions)
	actions[name] = a
	return Command{actions: actions, errs: c.errs}
}

func (c Command) withError(err error) Command {
	return Command{actions: c.actions, errs: append(slices.Clip(c.errs), err)}
}

// Main runs the command with os.Args and exits.
func (c Command) Main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := c.Run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}

// Run runs the command with the given arguments in the current directory
// and returns the exit code.
func (c Command) Run(ctx context.Context, args []string) int {
	if len(c.errs) > 0 {
		for _, err := range c.errs {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		return cli.ExitError
	}
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return cli.ExitError
	}
	app := &cli.App{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Dir:     dir,
		Environ: os.Environ(),
		Presets: c.actions,
	}
	return app.Main(ctx, args)
}
