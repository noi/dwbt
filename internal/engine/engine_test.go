package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerHint(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".dwbt")
	path := filepath.Join(dir, "workflows", "a.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("steps: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	actions, err := Builtin().Map()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Load(dir, dir, nil, actions)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct{ hint, want string }{
		{"", `server "api" is not defined; set it in config.yaml`},
		{"--server %[1]s=<url>", `server "api" is not defined; set it in config.yaml or override it with --server api=<url>`},
	} {
		r, err := plan.Runner(RunOptions{OverrideHint: tt.hint})
		if err != nil {
			t.Fatal(err)
		}
		_, err = r.Runtime.Server("api")
		if err == nil || err.Error() != tt.want {
			t.Errorf("hint %q: got %v, want %s", tt.hint, err, tt.want)
		}
	}

	r, err := plan.Runner(RunOptions{Servers: map[string]string{"api": "http://example.test"}})
	if err != nil {
		t.Fatal(err)
	}
	if url, err := r.Runtime.Server("api"); url != "http://example.test" || err != nil {
		t.Errorf("overridden server: %q, %v", url, err)
	}
	if !strings.HasSuffix(plan.Workflows[0].Name, "a.yaml") {
		t.Errorf("workflow name %q", plan.Workflows[0].Name)
	}
}
