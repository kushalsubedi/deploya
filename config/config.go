package config

// ProjectContext holds everything the detector discovers about a project.
type ProjectContext struct {
	Language    string // go, python, node, java, ruby, rust, unknown
	Runtime     string // e.g. "3.11", "20", "1.21"
	HasDocker   bool
	HasCompose  bool
	TestCommand string
	Cloud       string // aws, gcp, azure, none
	MainBranch  string // main or master
	RepoName    string
}
