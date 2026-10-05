# dwbt-gui

dwbt-gui is a desktop app to create, edit and run dwbt workflows and actions by arranging blocks. It is built with [Wails](https://wails.io) (Go + React).

## Features

- List, create, edit and save the workflows (`workflows/`) and actions (`actions/`) of a `.dwbt` directory
- Steps are cards stacked vertically, reordered by dragging or with ↑↓
- Add steps from a palette of actions; the parameters of an action are shown with their types and whether they are required
- The `{}` button offers the values a field usually takes and the references available there (`params`, `inputs`, `item`, `outputs.current` and so on). Picking an output of a preceding step adds an input receiving it and references that input
- The edited definition is validated as you type, with problems shown on their steps
- Run a workflow without saving it and see the result of each step, choosing the environment and overriding server URLs
- Preview of the generated YAML

Editing `config.yaml` is not supported.

Values are read as YAML: `20` is a number and `"20"` a string. Saving rewrites the YAML, so comments and formatting outside of values are not preserved.

## Development

Go and Node.js are required. The Wails CLI is registered as a tool in `gui/go.mod`, so it needs no separate installation (run it as `go tool wails` in `gui`).

Run make at the root of the repository:

```sh
make            # list the targets
make run        # install the frontend dependencies and run with hot reload
make run DIR=path/to/project   # choose the directory to open (examples/users by default)
make build      # build the app into gui/build/bin
make test       # run the Go tests of dwbt and the GUI, and type-check the frontend
```

`make run` opens examples/users by default, so saving rewrites the files of the example.

The app opens the `.dwbt` directory found in the directory given as its argument (the current directory by default) or one of its parents. If none is found, you can choose one in the app.

`gui` is a Go module separate from dwbt so that the dependencies on Wails do not leak into dwbt. It refers to dwbt in the parent directory with a `replace` directive.
