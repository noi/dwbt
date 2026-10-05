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
	"time"

	"github.com/noi/dwbt/action"
	"github.com/noi/dwbt/internal/check"
	"github.com/noi/dwbt/internal/def"
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
	proj, wfs, ok := a.load(paths)
	if !ok {
		return ExitError
	}
	fmt.Fprintf(a.Stdout, "ok: %d workflow(s), %d action(s)\n", len(wfs), len(proj.Actions))
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
	proj, wfs, ok := a.load(paths)
	if !ok {
		return ExitError
	}
	env := environ(a.Environ)
	rt, err := newRuntime(proj.Config, *envName, overrides, env)
	if err != nil {
		fmt.Fprintf(a.Stderr, "error: %v\n", err)
		return ExitError
	}

	r := &runner.Runner{
		Resolver: def.Resolver{Presets: a.Presets, Files: proj.Actions},
		Runtime:  rt,
		Env:      env,
	}
	var counts [4]int
	for _, wf := range wfs {
		res := r.Run(ctx, wf)
		counts[res.Status]++
		a.report(res)
	}
	fmt.Fprintf(a.Stdout, "\n%d workflow(s): %d passed, %d failed, %d errored\n",
		len(wfs), counts[runner.Passed], counts[runner.Failed], counts[runner.Errored])
	switch {
	case counts[runner.Errored] > 0:
		return ExitError
	case counts[runner.Failed] > 0:
		return ExitFailure
	}
	return ExitOK
}

// load loads the project and the workflows, and validates them.
func (a *App) load(paths []string) (*def.Project, []*def.Workflow, bool) {
	root, err := def.FindRoot(a.Dir)
	if err != nil {
		fmt.Fprintf(a.Stderr, "error: %v\n", err)
		return nil, nil, false
	}
	var errs def.Errors
	proj, err := def.Load(root, a.Dir)
	if proj == nil {
		fmt.Fprintf(a.Stderr, "error: %v\n", err)
		return nil, nil, false
	}
	errs = appendErr(errs, err)
	for _, w := range proj.Warnings {
		fmt.Fprintf(a.Stderr, "warning: %s\n", w)
	}

	files := proj.WorkflowFiles
	if len(paths) > 0 {
		files = nil
		for _, p := range paths {
			if !filepath.IsAbs(p) {
				p = filepath.Join(a.Dir, p)
			}
			p, _ = filepath.Abs(p)
			files = append(files, p)
		}
	}
	if len(files) == 0 && len(errs) == 0 {
		fmt.Fprintf(a.Stderr, "error: no workflows found in %s\n", filepath.Join(root, "workflows"))
		return nil, nil, false
	}
	var wfs []*def.Workflow
	for _, f := range files {
		wf, err := proj.LoadWorkflow(f)
		errs = appendErr(errs, err)
		if wf != nil && err == nil {
			wfs = append(wfs, wf)
		}
	}
	if len(errs) == 0 {
		errs = appendErr(errs, check.Check(proj, a.Presets, wfs))
	}
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(a.Stderr, "error: %v\n", e)
		}
		return nil, nil, false
	}
	return proj, wfs, true
}

func (a *App) report(res *runner.Result) {
	title := res.Workflow.Name
	if res.Workflow.Description != "" {
		title += " - " + res.Workflow.Description
	}
	fmt.Fprintf(a.Stdout, "=== %s\n", title)
	for _, s := range res.Steps {
		if s.Status == runner.Skipped {
			fmt.Fprintf(a.Stdout, "  %-5s %s\n", s.Status, s.Step.Label())
			continue
		}
		fmt.Fprintf(a.Stdout, "  %-5s %s  %s\n", s.Status, s.Step.Label(), duration(s.Duration))
		if s.Err != nil {
			for _, line := range strings.Split(s.Err.Error(), "\n") {
				fmt.Fprintf(a.Stdout, "        %s\n", line)
			}
		}
	}
	fmt.Fprintf(a.Stdout, "--- %s %s (%s)\n", res.Status, res.Workflow.Name, duration(res.Duration))
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

func environ(kvs []string) map[string]any {
	env := map[string]any{}
	for _, kv := range kvs {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	return env
}

func appendErr(errs def.Errors, err error) def.Errors {
	if err == nil {
		return errs
	}
	var list def.Errors
	if errors.As(err, &list) {
		return append(errs, list...)
	}
	return append(errs, err)
}

func duration(d time.Duration) string {
	if d < time.Millisecond {
		return d.Round(time.Microsecond).String()
	}
	return d.Round(time.Millisecond).String()
}
