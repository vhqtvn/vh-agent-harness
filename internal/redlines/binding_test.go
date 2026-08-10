package redlines

import (
	"os/exec"
	"testing"
)

// repoLookPathGit returns the git path or an error if git is not installed.
// Shared by the git-backed tests in this package.
func repoLookPathGit() (string, error) {
	return exec.LookPath("git")
}

// All identifiers and terms in this file are OBVIOUSLY synthetic. No real
// registry entry is represented here.

func TestNormalizeRemote_Reductions(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{"scp https with git suffix", "https://github.com/synthetic-org/synthetic-repo.git", "github.com/synthetic-org/synthetic-repo", true},
		{"scp ssh form", "git@github.com:synthetic-org/synthetic-repo.git", "github.com/synthetic-org/synthetic-repo", true},
		{"ssh scheme", "ssh://git@github.com/synthetic-org/synthetic-repo.git", "github.com/synthetic-org/synthetic-repo", true},
		{"https with userinfo stripped", "https://oauth2:TOKEN@github.com/synthetic-org/synthetic-repo.git", "github.com/synthetic-org/synthetic-repo", true},
		{"host lowercased path preserved", "https://GitHub.COM/Synthetic-Org/Synthetic-Repo", "github.com/Synthetic-Org/Synthetic-Repo", true},
		{"trailing slash stripped", "https://gitlab.example.com/g/synthetic.git/", "gitlab.example.com/g/synthetic", true},
		{"empty", "", "", false},
		{"local path absolute", "/home/me/repo", "", false},
		{"local path relative", "./repo", "", false},
		{"bare name", "synthetic-repo", "", false},
		{"scheme host no path", "https://github.com", "", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got, ok := NormalizeRemote(c.raw)
			if ok != c.ok {
				t.Fatalf("NormalizeRemote(%q): ok=%v want %v (got %q)", c.raw, ok, c.ok, got)
			}
			if ok && got != c.want {
				t.Errorf("NormalizeRemote(%q): got %q want %q", c.raw, got, c.want)
			}
		})
	}
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pat, name string
		want      bool
	}{
		{"/home/me/**", "/home/me/repo-a", true},
		{"/home/me/**", "/home/me/repo-a/sub", true},
		{"/home/me/**", "/home/other/repo", false},
		{"/home/me/*", "/home/me/repo-a", true},
		{"/home/me/*", "/home/me/repo-a/sub", false}, // * does not cross /
		{"github.com/synthetic-org/*", "github.com/synthetic-org/synthetic-repo", true},
		{"github.com/synthetic-org/*", "github.com/synthetic-org/synthetic-repo/sub", false},
		{"github.com/**/synthetic-repo", "github.com/synthetic-org/synthetic-repo", true},
		{"github.com/**/synthetic-repo", "github.com/a/b/synthetic-repo", true},
		{"**", "/anything/here", true},
		{"**", "github.com/x/y", true},
		{"exact", "exact", true},
		{"exact", "other", false},
		// doublestar matches zero segments too
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"a/**/b", "a/x/c", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.pat+"~"+c.name, func(t *testing.T) {
			if got := matchGlob(c.pat, c.name); got != c.want {
				t.Errorf("matchGlob(%q,%q)=%v want %v", c.pat, c.name, got, c.want)
			}
		})
	}
}

func TestSubject_Binds(t *testing.T) {
	const path = "/home/me/synthetic-repo"
	remotes := []string{"github.com/synthetic-org/synthetic-repo"}

	cases := []struct {
		name string
		subj Subject
		want bool
	}{
		{
			name: "empty repos binds all",
			subj: Subject{ID: "subj-test-scrub", Kind: KindScrubProject, Repos: nil},
			want: true,
		},
		{
			name: "path glob match",
			subj: Subject{ID: "subj-test-scrub", Kind: KindScrubProject, Repos: []string{"/home/me/**"}},
			want: true,
		},
		{
			name: "remote glob match",
			subj: Subject{ID: "subj-test-scrub", Kind: KindScrubProject, Repos: []string{"github.com/synthetic-org/*"}},
			want: true,
		},
		{
			name: "neither matches",
			subj: Subject{ID: "subj-test-scrub", Kind: KindScrubProject, Repos: []string{"/other/**", "gitlab.example.com/*"}},
			want: false,
		},
		{
			name: "non-absolute path never path-matches",
			subj: Subject{ID: "subj-test-scrub", Kind: KindScrubProject, Repos: []string{"repo"}},
			want: false,
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if got := c.subj.Binds(path, remotes); got != c.want {
				t.Errorf("Binds=%v want %v", got, c.want)
			}
		})
	}
}

func TestSubject_IsAmbient(t *testing.T) {
	rel := Subject{
		ID: "subj-test-relation", Kind: KindForbiddenRelation,
		SideA: []string{"synthetic-alpha"}, SideB: []string{"synthetic-beta"},
		AmbientRepos: []string{"github.com/synthetic-org/*"},
	}
	const path = "/home/me/synthetic-repo"
	if !rel.IsAmbient(path, []string{"github.com/synthetic-org/synthetic-repo"}) {
		t.Error("expected ambient via remote")
	}
	if rel.IsAmbient(path, []string{"gitlab.example.com/other/repo"}) {
		t.Error("ambient should not fire for non-matching remote")
	}
	// Empty AmbientRepos => never ambient.
	rel2 := rel
	rel2.AmbientRepos = nil
	if rel2.IsAmbient(path, []string{"github.com/synthetic-org/synthetic-repo"}) {
		t.Error("ambient should be false with no ambient_repos")
	}
	// Non-relation kind => never ambient.
	scrub := Subject{ID: "subj-test-scrub", Kind: KindScrubProject, AmbientRepos: []string{"**"}}
	if scrub.IsAmbient(path, []string{"github.com/synthetic-org/synthetic-repo"}) {
		t.Error("scrub-project must never be ambient")
	}
}

func TestSubject_IsSource(t *testing.T) {
	scrub := Subject{
		ID: "subj-test-scrub", Kind: KindScrubProject,
		Labels:      []string{"synthetic-alpha"},
		SourceRepos: []string{"/home/me/sensitive/**"},
	}
	if !scrub.IsSource("/home/me/sensitive/origin", nil) {
		t.Error("expected source via path")
	}
	if scrub.IsSource("/home/me/elsewhere", nil) {
		t.Error("source should not fire for non-matching path")
	}
	// Empty SourceRepos => never source.
	scrub2 := scrub
	scrub2.SourceRepos = nil
	if scrub2.IsSource("/home/me/sensitive/origin", nil) {
		t.Error("source should be false with no source_repos")
	}
	// Non-scrub kind => never source.
	rel := Subject{ID: "subj-test-relation", Kind: KindForbiddenRelation, SourceRepos: []string{"**"}}
	if rel.IsSource("/home/me/anywhere", nil) {
		t.Error("forbidden-relation must never be source")
	}
}

func TestRepoRemotes_RealGitRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git-backed test in -short")
	}
	if _, err := repoLookPathGit(); err != nil {
		t.Skipf("git not available: %v", err)
	}
	repo := gitInitRepo(t)
	// Add a synthetic remote and confirm it round-trips through normalization.
	gitRun(t, repo, "remote", "add", "origin", "https://github.com/synthetic-org/synthetic-repo.git")

	remotes, err := RepoRemotes(repo)
	if err != nil {
		t.Fatalf("RepoRemotes: %v", err)
	}
	want := "github.com/synthetic-org/synthetic-repo"
	found := false
	for _, r := range remotes {
		if r == want {
			found = true
		}
	}
	if !found {
		t.Errorf("RepoRemotes missing %q; got %v", want, remotes)
	}
}
