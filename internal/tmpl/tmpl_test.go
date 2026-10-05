package tmpl

import (
	"reflect"
	"strings"
	"testing"

	"github.com/noi/dwbt/internal/yamlx"
)

var pos = yamlx.Pos{File: "test.yaml", Line: 1, Col: 1}

func TestTemplateEval(t *testing.T) {
	env := Env{
		"inputs": map[string]any{
			"id":    3,
			"name":  "alice",
			"user":  map[string]any{"id": 3},
			"ratio": 1.5,
		},
	}
	tests := []struct {
		src  string
		want any
	}{
		{"plain", "plain"},
		{"", ""},
		{"<<inputs.id>>", 3},
		{"<< inputs.id >>", 3},
		{"<<inputs.user>>", map[string]any{"id": 3}},
		{"/users/<<inputs.id>>/items", "/users/3/items"},
		{"<<inputs.name>>-<<inputs.id>>", "alice-3"},
		{"u=<<inputs.user>>", `u={"id":3}`},
		{"r=<<inputs.ratio>>", "r=1.5"},
		{"n=<<nil>>", "n=null"},
		{`<<"a>>b">>`, "a>>b"},
		{`<<"<<">>`, "<<"},
		{`x <<'>>'>> y`, "x >> y"},
		{"a > b", "a > b"},
	}
	for _, tt := range tests {
		tp, err := Parse(tt.src, pos)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.src, err)
			continue
		}
		got, err := tp.Eval(env)
		if err != nil {
			t.Errorf("Eval(%q): %v", tt.src, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Eval(%q) = %#v, want %#v", tt.src, got, tt.want)
		}
	}
}

func TestTemplateParseErrors(t *testing.T) {
	tests := map[string]string{
		"<<inputs.id":     "unclosed <<",
		`<<"abc>>`:        "unterminated string",
		"<<>>":            "empty expression",
		"<<inputs.(>>":    "invalid expression",
		"ok <<1 +>> rest": "invalid expression",
	}
	for src, want := range tests {
		_, err := Parse(src, pos)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%q) error = %v, want containing %q", src, err, want)
		}
	}
}

func TestRefs(t *testing.T) {
	e, err := Compile(`any(outputs.current.res.body, .id == inputs.users[0].id) && len(params.name) > 0 && uuid() != "" && (let x = 1; x == index)`, pos)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range e.Refs() {
		got = append(got, r.Root+":"+strings.Join(r.Path, "."))
	}
	want := []string{"outputs:current.res.body", "inputs:users", "params:name", "index:"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("refs = %v, want %v", got, want)
	}
}

func TestExplain(t *testing.T) {
	e, err := Compile(`any(outputs.current.body, .id == inputs.users[0].id)`, pos)
	if err != nil {
		t.Fatal(err)
	}
	env := Env{
		"outputs": map[string]any{"current": map[string]any{"body": []any{map[string]any{"id": 5}}}},
		"inputs":  map[string]any{"users": []any{map[string]any{"id": 3}}},
	}
	got := e.Explain(env)
	want := []string{`outputs.current.body = [{"id":5}]`, `inputs.users[0].id = 3`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Explain = %q, want %q", got, want)
	}
}

func TestUUID(t *testing.T) {
	e, err := Compile(`uuid()`, pos)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.Eval(Env{})
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := v.(string); len(s) != 36 || s[14] != '4' {
		t.Errorf("uuid() = %v", v)
	}
}
