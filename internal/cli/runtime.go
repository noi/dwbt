package cli

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/noi/dwbt/internal/def"
	"github.com/noi/dwbt/internal/tmpl"
	"github.com/noi/dwbt/internal/value"
)

// runtime implements action.Runtime.
type runtime struct {
	env     string
	servers map[string]server
	timeout time.Duration
}

type server struct {
	url string
	err error
}

// newRuntime resolves the server URLs. A server given with --server takes
// precedence over the profile selected with --env, which takes precedence
// over the default profile.
func newRuntime(cfg *def.Config, envName string, overrides map[string]string, env map[string]any) (*runtime, error) {
	rt := &runtime{servers: map[string]server{}, timeout: cfg.HTTPTimeout}
	if envName != "" && cfg.Environments[envName] == nil {
		names := slices.Sorted(maps.Keys(cfg.Environments))
		if len(names) == 0 {
			return nil, fmt.Errorf("environment %q is not defined: config.yaml has no environments", envName)
		}
		return nil, fmt.Errorf("environment %q is not defined (available: %s)", envName, strings.Join(names, ", "))
	}
	dflt := cfg.Default
	if dflt == "" && len(cfg.Environments) == 1 {
		for name := range cfg.Environments {
			dflt = name
		}
	}
	rt.env = envName
	if rt.env == "" {
		rt.env = dflt
	}

	vars := tmpl.Env{"env": env}
	apply := func(name string) {
		for id, t := range cfg.Environments[name].Servers {
			v, err := t.Eval(vars)
			switch {
			case err != nil:
				rt.servers[id] = server{err: err}
			case v == nil || v == "":
				rt.servers[id] = server{err: fmt.Errorf("%s: URL of server %q is empty", t.Pos, id)}
			default:
				rt.servers[id] = server{url: value.String(v)}
			}
		}
	}
	if dflt != "" && dflt != rt.env {
		apply(dflt)
	}
	if rt.env != "" {
		apply(rt.env)
	}
	for id, url := range overrides {
		rt.servers[id] = server{url: url}
	}
	return rt, nil
}

func (rt *runtime) Server(id string) (string, error) {
	s, ok := rt.servers[id]
	if !ok {
		if rt.env == "" {
			return "", fmt.Errorf("server %q is not defined; set it in config.yaml or with --server %s=<url>", id, id)
		}
		return "", fmt.Errorf("server %q is not defined in environment %q", id, rt.env)
	}
	return s.url, s.err
}

func (rt *runtime) HTTPTimeout() time.Duration { return rt.timeout }
