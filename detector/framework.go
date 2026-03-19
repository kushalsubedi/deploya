package detector

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type packageJSONFull struct {
	Scripts      map[string]string `json:"scripts"`
	Dependencies map[string]string `json:"dependencies"`
	DevDeps      map[string]string `json:"devDependencies"`
}

// DetectFramework detects the JS/TS framework and returns framework name + build command.
// Only meaningful when lang == "node".
func DetectFramework(dir string) (framework, buildCommand string) {
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return "plain", ""
	}

	var pkg packageJSONFull
	if err := json.Unmarshal(b, &pkg); err != nil {
		return "plain", ""
	}

	// Merge deps + devDeps for detection
	allDeps := map[string]string{}
	for k, v := range pkg.Dependencies {
		allDeps[k] = v
	}
	for k, v := range pkg.DevDeps {
		allDeps[k] = v
	}

	// Detect framework from dependencies
	switch {
	case hasDep(allDeps, "next"):
		return "nextjs", "npm run build"
	case hasDep(allDeps, "react"):
		return "react", "npm run build"
	case hasDep(allDeps, "vue"):
		return "vue", "npm run build"
	case hasDep(allDeps, "@sveltejs/kit") || hasDep(allDeps, "svelte"):
		return "svelte", "npm run build"
	case hasDep(allDeps, "@angular/core"):
		return "angular", "npm run build"
	case hasDep(allDeps, "express") || hasDep(allDeps, "fastify") || hasDep(allDeps, "koa"):
		return "express", "" // backend — no build step needed
	}

	// Check if build script exists in package.json
	if _, ok := pkg.Scripts["build"]; ok {
		return "plain", "npm run build"
	}

	return "plain", ""
}

func hasDep(deps map[string]string, name string) bool {
	_, ok := deps[name]
	return ok
}
