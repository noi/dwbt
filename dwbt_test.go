package dwbt_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/noi/dwbt"
	"github.com/noi/dwbt/action"
)

// upper is an action implemented in Go.
type upper struct{}

func (upper) Params() action.ParamSpec {
	return action.ParamSpec{"text": {Type: action.String, Required: true}}
}

func (upper) Outputs() []string { return []string{"text"} }

func (upper) Run(_ context.Context, _ action.Runtime, params map[string]any) (map[string]any, error) {
	s := params["text"].(string)
	b := []byte(s)
	for i, c := range b {
		if 'a' <= c && c <= 'z' {
			b[i] = c - 'a' + 'A'
		}
	}
	return map[string]any{"text": string(b)}, nil
}

func TestAction(t *testing.T) {
	dir := t.TempDir()
	wf := `
steps:
  - id: up
    use: text/upper
    foreach: [abc, xyz]
    params:
      text: <<item>>
    expects:
      - asserts:
          - outputs.current.text == upper(item)
    outputs:
      texts: <<outputs.current.text>>
  - use: text/upper
    inputs:
      texts: <<outputs.up.texts>>
    params:
      text: <<join(inputs.texts, ",")>>
    expects:
      - asserts:
          - outputs.current.text == "ABC,XYZ"
`
	path := filepath.Join(dir, ".dwbt", "workflows", "a.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(wf), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	ctx := context.Background()
	base := dwbt.New()
	withUpper := base.Action("text/upper", upper{})
	if code := withUpper.Run(ctx, []string{"run"}); code != 0 {
		t.Errorf("exit %d", code)
	}
	// Action does not modify the receiver.
	if code := base.Run(ctx, []string{"validate"}); code != 2 {
		t.Errorf("without the action: exit %d, want 2", code)
	}
	if code := withUpper.Action("http", upper{}).Run(ctx, []string{"validate"}); code != 2 {
		t.Errorf("replacing http: exit %d, want 2", code)
	}
	if code := withUpper.Action("Bad Name", upper{}).Run(ctx, []string{"validate"}); code != 2 {
		t.Errorf("invalid name: exit %d, want 2", code)
	}
	if code := withUpper.Run(ctx, []string{"validate"}); code != 0 {
		t.Errorf("errors leaked into the receiver: exit %d", code)
	}
}
