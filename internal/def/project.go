package def

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/noi/dwbt/internal/yamlx"
)

// DirName is the name of the directory holding definitions.
const DirName = ".dwbt"

// Project is a loaded .dwbt directory.
type Project struct {
	// Dir is the path of the .dwbt directory.
	Dir string
	// Base is the directory used to display file names, usually the
	// current directory.
	Base    string
	Config  *Config
	Actions map[string]*ActionDef
	// WorkflowFiles lists the workflow files under workflows/, sorted.
	WorkflowFiles []string
	Warnings      []string
}

// FindRoot searches for a .dwbt directory in start and its parents. The
// first one found is used.
func FindRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(dir, DirName)
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%s directory not found in %s or any parent directory", DirName, start)
		}
		dir = parent
	}
}

// Load loads config.yaml and the actions of the .dwbt directory dir, and
// lists its workflow files. base is used to display file names.
func Load(dir, base string) (*Project, error) {
	if abs, err := filepath.Abs(base); err == nil {
		base = abs
	}
	p := &Project{Dir: dir, Base: base, Actions: map[string]*ActionDef{}}
	var errs Errors

	cfgPath := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		cfg, err := p.loadConfig(cfgPath)
		if err != nil {
			errs = append(errs, flatten(err)...)
		}
		p.Config = cfg
	} else {
		p.warnYML(filepath.Join(dir, "config.yml"))
	}
	if p.Config == nil {
		p.Config = &Config{HTTPTimeout: DefaultHTTPTimeout, Environments: map[string]*Environment{}}
	}

	actionsDir := filepath.Join(dir, "actions")
	files, err := p.yamlFiles(actionsDir)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		rel, _ := filepath.Rel(actionsDir, f)
		name := filepath.ToSlash(strings.TrimSuffix(rel, ".yaml"))
		if !IsActionName(name) {
			errs = append(errs, yamlx.Errorf(yamlx.Pos{File: p.display(f)}, "invalid action name %q derived from the file path", name))
			continue
		}
		n, err := yamlx.ParseFile(f, p.display(f))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		def, err := ParseAction(n, name)
		if err != nil {
			errs = append(errs, flatten(err)...)
		}
		p.Actions[name] = def
	}

	p.WorkflowFiles, err = p.yamlFiles(filepath.Join(dir, "workflows"))
	if err != nil {
		return nil, err
	}
	return p, errs.Err()
}

// LoadWorkflow loads the workflow file at path.
func (p *Project) LoadWorkflow(path string) (*Workflow, error) {
	if !strings.HasSuffix(path, ".yaml") {
		return nil, fmt.Errorf("%s: workflow files must have the .yaml extension", p.display(path))
	}
	n, err := yamlx.ParseFile(path, p.display(path))
	if err != nil {
		return nil, err
	}
	wf, err := ParseWorkflow(n, p.display(path))
	if wf != nil {
		wf.Path = path
	}
	return wf, err
}

func (p *Project) loadConfig(path string) (*Config, error) {
	n, err := yamlx.ParseFile(path, p.display(path))
	if err != nil {
		return nil, err
	}
	return ParseConfig(n)
}

// yamlFiles lists the .yaml files under dir recursively. Files with the .yml
// extension are reported as warnings since they are not loaded.
func (p *Project) yamlFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch filepath.Ext(path) {
		case ".yaml":
			files = append(files, path)
		case ".yml":
			p.warnYML(path)
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	slices.Sort(files)
	return files, err
}

func (p *Project) warnYML(path string) {
	if _, err := os.Stat(path); err == nil {
		p.Warnings = append(p.Warnings, fmt.Sprintf("%s: ignored; use the .yaml extension", p.display(path)))
	}
}

func (p *Project) display(path string) string {
	if p.Base == "" {
		return path
	}
	if rel, err := filepath.Rel(p.Base, path); err == nil {
		return rel
	}
	return path
}

func flatten(err error) []error {
	var errs Errors
	if errors.As(err, &errs) {
		return errs
	}
	return []error{err}
}
