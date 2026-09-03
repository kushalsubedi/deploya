package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// runValidate checks every workflow under .github/workflows for YAML
// syntax errors and common structural mistakes (missing jobs, steps,
// runs-on, or needs pointing at jobs that don't exist).
func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	dir := fs.String("dir", ".", "Project directory")
	fs.Usage = func() {
		fmt.Println(`Usage: deploya validate [flags]

Validates the workflow files in .github/workflows.

Flags:`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	workflowDir := filepath.Join(*dir, ".github", "workflows")
	entries, err := os.ReadDir(workflowDir)
	if err != nil {
		return fmt.Errorf("no workflows found at %s — run 'deploya init' first", workflowDir)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext == ".yml" || ext == ".yaml" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	if len(files) == 0 {
		return fmt.Errorf("no .yml workflow files in %s", workflowDir)
	}

	failed := 0
	for _, name := range files {
		path := filepath.Join(workflowDir, name)
		problems := validateWorkflow(path)
		if len(problems) == 0 {
			fmt.Printf("✅ %s\n", name)
			continue
		}
		failed++
		fmt.Printf("❌ %s\n", name)
		for _, p := range problems {
			fmt.Printf("   • %s\n", p)
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d of %d workflow file(s) failed validation", failed, len(files))
	}
	fmt.Printf("\nAll %d workflow file(s) look good 🎉\n", len(files))
	return nil
}

type workflowFile struct {
	Name string                 `yaml:"name"`
	On   interface{}            `yaml:"on"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	RunsOn interface{}              `yaml:"runs-on"`
	Uses   string                   `yaml:"uses"`
	Needs  interface{}              `yaml:"needs"`
	Steps  []map[string]interface{} `yaml:"steps"`
}

func validateWorkflow(path string) []string {
	var problems []string

	b, err := os.ReadFile(path)
	if err != nil {
		return []string{fmt.Sprintf("could not read file: %v", err)}
	}

	var wf workflowFile
	if err := yaml.Unmarshal(b, &wf); err != nil {
		return []string{fmt.Sprintf("invalid YAML: %v", err)}
	}

	if wf.On == nil {
		problems = append(problems, "missing 'on:' trigger")
	}
	if len(wf.Jobs) == 0 {
		problems = append(problems, "no jobs defined")
	}

	for jobName, job := range wf.Jobs {
		if job.Uses == "" {
			if job.RunsOn == nil {
				problems = append(problems, fmt.Sprintf("job '%s': missing runs-on", jobName))
			}
			if len(job.Steps) == 0 {
				problems = append(problems, fmt.Sprintf("job '%s': no steps", jobName))
			}
		}
		for _, need := range needsList(job.Needs) {
			if _, ok := wf.Jobs[need]; !ok {
				problems = append(problems, fmt.Sprintf("job '%s': needs '%s' which does not exist", jobName, need))
			}
		}
	}

	return problems
}

func needsList(needs interface{}) []string {
	switch v := needs.(type) {
	case string:
		return []string{v}
	case []interface{}:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
