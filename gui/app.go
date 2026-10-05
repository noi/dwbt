package main

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/noi/dwbt/gui/internal/doc"
	"github.com/noi/dwbt/gui/internal/studio"
)

// App is bound to the frontend. Its methods wrap those of studio.Studio.
type App struct {
	ctx    context.Context
	studio *studio.Studio
	// dir is the directory the project is first searched from.
	dir string
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Initial opens the project found from the starting directory, or returns
// nil if there is none.
func (a *App) Initial() *studio.Project {
	p, err := a.studio.Open(a.dir)
	if err != nil {
		return nil
	}
	return p
}

// Choose lets the user choose a directory and opens the project found from
// it. It returns nil if the user cancels.
func (a *App) Choose() (*studio.Project, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Open a directory containing .dwbt",
		CanCreateDirectories: false,
	})
	if err != nil || dir == "" {
		return nil, err
	}
	return a.studio.Open(dir)
}

func (a *App) Project() (*studio.Project, error) { return a.studio.Project() }

func (a *App) Workflow(name string) (*doc.Workflow, error) { return a.studio.Workflow(name) }

func (a *App) SaveWorkflow(name string, wf *doc.Workflow) error {
	return a.studio.SaveWorkflow(name, wf)
}

func (a *App) Action(name string) (*doc.Action, error) { return a.studio.Action(name) }

func (a *App) SaveAction(name string, act *doc.Action) error {
	return a.studio.SaveAction(name, act)
}

func (a *App) RenderWorkflow(wf *doc.Workflow) (string, error) { return a.studio.RenderWorkflow(wf) }

func (a *App) RenderAction(act *doc.Action) (string, error) { return a.studio.RenderAction(act) }

func (a *App) CheckWorkflow(name string, wf *doc.Workflow) []*studio.Problem {
	return a.studio.CheckWorkflow(name, wf)
}

func (a *App) CheckAction(name string, act *doc.Action) []*studio.Problem {
	return a.studio.CheckAction(name, act)
}

func (a *App) Run(name string, wf *doc.Workflow, opts studio.RunOptions) (*studio.RunResult, error) {
	if a.ctx == nil {
		return nil, errors.New("the application is not started")
	}
	return a.studio.Run(a.ctx, name, wf, opts)
}

func (a *App) Stop() { a.studio.Stop() }
