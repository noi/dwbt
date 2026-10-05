// Package cli implements the dwbt command.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/internal/def"
	"github.com/noi/dwbt/internal/engine"
	"github.com/noi/dwbt/internal/runner"
)

// Exit codes.
const (
	ExitOK      = 0
	ExitFailure = 1 // an expectation was not met
	ExitError   = 2 // anything else
)

const usage = `Usage:
  dwbt validate [workflow...]
  dwbt run      [workflow...] [--env <name>] [--server <id>=<url>]...

Workflows default to all files under .dwbt/workflows.
`

// App is the dwbt command.
type App struct {
	Stdout, Stderr io.Writer
	// Dir is the working directory.
	Dir string
	// Environ holds the environment variables, as from os.Environ.
	Environ []string
	Presets map[string]action.Action
}

// Main runs the command with the given arguments and returns the exit code.
func (a *App) Main(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(a.Stderr, usage)
		return ExitError
	}
	switch args[0] {
	case "validate":
		return a.validate(args[1:])
	case "run":
		return a.run(ctx, args[1:])
	case "help", "-h", "--help":
		fmt.Fprint(a.Stdout, usage)
		return ExitOK
	default:
		fmt.Fprintf(a.Stderr, "unknown command %q\n\n%s", args[0], usage)
		return ExitError
	}
}

func (a *App) validate(args []string) int {
	fs := a.flagSet("validate")
	paths, err := parseArgs(fs, args)
	if err != nil {
		return ExitError
	}
	plan, ok := a.load(paths)
	if !ok {
		return ExitError
	}
	fmt.Fprintf(a.Stdout, "ok: %d workflow(s), %d action(s)\n", len(plan.Workflows), len(plan.Project.Actions))
	return ExitOK
}

func (a *App) run(ctx context.Context, args []string) int {
	fs := a.flagSet("run")
	envName := fs.String("env", "", "environment profile in config.yaml")
	overrides := serverFlags{}
	fs.Var(overrides, "server", "override a server URL, as `id=url` (repeatable)")
	paths, err := parseArgs(fs, args)
	if err != nil {
		return ExitError
	}
	plan, ok := a.load(paths)
	if !ok {
		return ExitError
	}
	r, err := plan.Runner(engine.RunOptions{
		Env:          *envName,
		Servers:      overrides,
		Environ:      a.Environ,
		OverrideHint: "--server %[1]s=<url>",
	})
	if err != nil {
		fmt.Fprintf(a.Stderr, "error: %v\n", err)
		return ExitError
	}

	var counts [4]int
	for _, wf := range plan.Workflows {
		res := r.Run(ctx, wf)
		counts[res.Status]++
		a.report(res)
	}
	fmt.Fprintf(a.Stdout, "\n%d workflow(s): %d passed, %d failed, %d errored\n",
		len(plan.Workflows), counts[runner.Passed], counts[runner.Failed], counts[runner.Errored])
	switch {
	case counts[runner.Errored] > 0:
		return ExitError
	case counts[runner.Failed] > 0:
		return ExitFailure
	}
	return ExitOK
}

// load loads the project and the workflows, and validates them.
func (a *App) load(paths []string) (*engine.Plan, bool) {
	root, err := def.FindRoot(a.Dir)
	if err != nil {
		fmt.Fprintf(a.Stderr, "error: %v\n", err)
		return nil, false
	}
	var files []string
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			p = filepath.Join(a.Dir, p)
		}
		p, _ = filepath.Abs(p)
		files = append(files, p)
	}
	plan, err := engine.Load(root, a.Dir, files, a.Presets)
	if plan != nil {
		for _, w := range plan.Project.Warnings {
			fmt.Fprintf(a.Stderr, "warning: %s\n", w)
		}
	}
	if err != nil {
		for _, e := range engine.Errors(err) {
			fmt.Fprintf(a.Stderr, "error: %v\n", e)
		}
		return nil, false
	}
	return plan, true
}

func (a *App) report(res *runner.Result) {
	title := res.Workflow.Name
	if res.Workflow.Description != "" {
		title += " - " + res.Workflow.Description
	}
	fmt.Fprintf(a.Stdout, "=== %s\n", title)
	engine.WriteSteps(a.Stdout, res)
	fmt.Fprintf(a.Stdout, "--- %s %s (%s)\n", res.Status, res.Workflow.Name, engine.Duration(res.Duration))
}

func (a *App) flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.Usage = func() {
		fmt.Fprint(a.Stderr, usage)
		fs.PrintDefaults()
	}
	return fs
}

// parseArgs parses flags that may appear before, between or after the
// positional arguments.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

type serverFlags map[string]string

func (s serverFlags) String() string { return "" }

func (s serverFlags) Set(v string) error {
	id, url, ok := strings.Cut(v, "=")
	if !ok || id == "" || url == "" {
		return errors.New("must be id=url")
	}
	s[id] = url
	return nil
}
