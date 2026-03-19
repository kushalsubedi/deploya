package config

// ProjectContext holds everything the detector discovers about a project.
type ProjectContext struct {
	// Detected fields
	Language     string // go, python, node, java, ruby, rust, unknown
	Runtime      string // e.g. "3.11", "20", "1.21"
	Framework    string // react, nextjs, vue, svelte, angular, express, plain
	HasDocker    bool
	HasCompose   bool
	TestCommand  string
	BuildCommand string
	Cloud        string // aws, gcp, azure, none
	MainBranch   string // main or master
	RepoName     string

	// User choices
	Registry string // ghcr, dockerhub, ecr, gcr
	Notify   string // slack, discord, email, none
}
