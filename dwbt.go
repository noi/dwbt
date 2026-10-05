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
	"os"
	"os/signal"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/internal/cli"
	"github.com/noi/dwbt/internal/engine"
)

// Command is the dwbt command. Its methods return a new Command and leave
// the receiver unchanged, so a Command can be shared and extended safely.
type Command struct {
	actions engine.Actions
}

// New returns the dwbt command with the built-in actions.
func New() Command {
	return Command{actions: engine.Builtin()}
}

// Action registers an action implemented in Go under name, such as
// "db/seed". It cannot replace a built-in action or an action registered
// before.
func (c Command) Action(name string, a action.Action) Command {
	return Command{actions: c.actions.With(name, a)}
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
	actions, err := c.actions.Map()
	if err != nil {
		for _, err := range engine.Errors(err) {
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
		Presets: actions,
	}
	return app.Main(ctx, args)
}
