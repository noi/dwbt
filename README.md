<h1>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-wordmark-dark.svg">
    <img src="assets/logo-wordmark.svg" alt="dwbt" width="290">
  </picture>
</h1>

**D**efined **W**orkflow **B**ased **T**ests — pronounced "doubt"

dwbt is a tool for writing E2E tests as YAML workflows and running them.

> [!NOTE]
> dwbt is under active development. Features and usage are not yet stable.

For the specification, see [the specification comment in Issue #1](https://github.com/noi/dwbt/issues/1).

## Installation

```sh
go install github.com/noi/dwbt/cmd/dwbt@latest
```

## Usage

```text
dwbt validate [workflow...]
dwbt run      [workflow...] [--env <name>] [--server <id>=<url>]... [--parallel <n>]
```

dwbt searches for a `.dwbt` directory starting from the current directory and walking up through its parents, and loads the definitions inside it.

```text
.dwbt/
  config.yaml          # environment profiles
  actions/
    user/create.yaml   # → use: user/create
  workflows/
    follow.yaml
```

A server URL can be an `http://` or `https://` URL, or the path to a Unix domain socket in the form `unix:///path/to/app.sock` (absolute path) or `unix:app.sock` (relative to the current directory).

```yaml
environments:
  local:
    servers:
      api: unix:///tmp/app.sock
```

Workflows run one by one by default. `--parallel <n>` runs up to `n` workflows at the same time, and reports each of them as it finishes. Workflows that run in parallel must not depend on each other's data, and actions implemented in Go may be called concurrently.

The exit code is `0` on success, `1` when `expects` do not match, and `2` for any other error.

See [examples/users](examples/users) for an example.

## GUI

[gui](gui) provides a desktop app to create, edit and run workflows visually. Run `make run` at the root of the repository to start it.

## Extending actions in Go

You can use dwbt as a library to build a binary with actions implemented in Go.

```go
package main

import "github.com/noi/dwbt"

func main() {
	dwbt.New().Action("db/seed", seedAction{}).Main()
}
```

Actions implement the [`action.Action`](action/action.go) interface.

## Running from Go tests

If the system under test is written in Go, you can use the [`dwbttest`](dwbttest) package to run workflows from `go test`.

```go
func TestE2E(t *testing.T) {
	srv := httptest.NewServer(app.NewHandler())
	defer srv.Close()

	dwbttest.New("testdata/.dwbt").
		Server("api", srv.URL).
		Action("db/seed", seedAction{}).
		Run(t)
}
```

`New` takes the path to a `.dwbt` directory. It validates the definitions and then runs each workflow as a subtest, so you can select workflows with `go test -run TestE2E/follow.yaml`. Each method returns a new value without modifying its receiver, so you can share common settings across multiple tests.

`Parallel(n)` runs up to `n` workflows at the same time, like `--parallel` of the dwbt command. `Run` still returns after all the workflows finish, so a server closed by `defer` stays available to them.

## License

[MIT](LICENSE)
