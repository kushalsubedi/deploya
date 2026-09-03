package releaserc

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"v1.2.3", "v1.2.3", false},
		{"1.2.3", "1.2.3", false},
		{"v0.0.0", "v0.0.0", false},
		{"1.2", "", true},
		{"abc", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		v, err := ParseVersion(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseVersion(%q): expected error, got %v", tt.in, v)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseVersion(%q): unexpected error %v", tt.in, err)
			continue
		}
		if v.String() != tt.want {
			t.Errorf("ParseVersion(%q) = %q, want %q", tt.in, v.String(), tt.want)
		}
	}
}

func TestBump(t *testing.T) {
	v, _ := ParseVersion("v1.2.3")
	tests := []struct {
		bump BumpType
		want string
	}{
		{BumpMajor, "v2.0.0"},
		{BumpMinor, "v1.3.0"},
		{BumpPatch, "v1.2.4"},
		{BumpNone, "v1.2.3"},
	}
	for _, tt := range tests {
		if got := v.Bump(tt.bump).String(); got != tt.want {
			t.Errorf("Bump(%s) = %q, want %q", tt.bump, got, tt.want)
		}
	}
}

func TestDetermineBump(t *testing.T) {
	tests := []struct {
		name    string
		commits []CommitInfo
		want    BumpType
	}{
		{"feature commit", []CommitInfo{{Title: "feat: add stuff"}}, BumpMinor},
		{"fix commit", []CommitInfo{{Title: "fix: bug"}}, BumpPatch},
		{"breaking marker", []CommitInfo{{Title: "feat!: new api"}}, BumpMajor},
		{"breaking text", []CommitInfo{{Title: "feat: BREAKING CHANGE in api"}}, BumpMajor},
		{"breaking label", []CommitInfo{{Title: "feat: x", Label: "breaking"}}, BumpMajor},
		{"label wins over prefix", []CommitInfo{{Title: "chore: tidy", Label: "enhancement"}}, BumpMinor},
		{"mixed takes highest", []CommitInfo{{Title: "docs: readme"}, {Title: "feat: thing"}}, BumpMinor},
		{"unknown defaults patch", []CommitInfo{{Title: "whatever"}}, BumpPatch},
	}
	for _, tt := range tests {
		if got := DetermineBump(tt.commits); got != tt.want {
			t.Errorf("%s: DetermineBump = %s, want %s", tt.name, got, tt.want)
		}
	}
}
