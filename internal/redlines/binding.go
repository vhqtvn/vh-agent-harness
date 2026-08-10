package redlines

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// RepoRemotes runs `git -C <repoRoot> remote -v` (READ-ONLY) and returns the
// de-duplicated set of NORMALIZED remote identifiers for the repo. Both fetch
// and push URLs are considered; a remote with distinct fetch/push URLs
// contributes both. The repo's identity is taken to be the union, so a clone
// is recognized as long as ANY of its remotes matches a subject's remote glob.
//
// Normalized form (per NormalizeRemote): canonical "host/owner/repo[.git
// stripped]", lowercased host. This form SURVIVES RE-CLONES (the remote URL is
// the clone-stable identity, unlike a local filesystem path).
//
// Returns an error (never a panic) if repoRoot is not a git repo or git is
// unavailable. A repo with NO remotes returns an empty slice and nil error.
func RepoRemotes(repoRoot string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "remote", "-v")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("redlines: git remote -v in %q: %w", repoRoot, err)
	}
	seen := map[string]struct{}{}
	var remotes []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// `git remote -v` rows are "<name>\t<url> (<kind>)".
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		raw := fields[1]
		norm, ok := NormalizeRemote(raw)
		if !ok {
			continue // local path remote or unparseable; skip (path matchers cover it)
		}
		if _, dup := seen[norm]; dup {
			continue
		}
		seen[norm] = struct{}{}
		remotes = append(remotes, norm)
	}
	return remotes, nil
}

// scpLikeRemoteRe matches the SCP-style "git@github.com:owner/repo.git" form.
// It captures the host and the path-after-colon.
var scpLikeRemoteRe = regexp.MustCompile(`^(?P<user>[^@:/]+)@(?P<host>[^:/]+):(?P<path>.+)$`)

// NormalizeRemote parses a git remote URL and returns its canonical normalized
// form: "host/owner/repo" with any trailing slashes and the ".git" suffix
// stripped and the host lowercased. Owner/repo case is PRESERVED (some
// self-hosted servers are case-sensitive; GitHub/GitLab are not, but preserving
// case is the conservative, lossless choice — operators write their remote
// globs in the case their server uses).
//
// Supported inputs (all reduce to the same canonical form):
//
//	git@github.com:vhqtvn/vh-agent-harness.git   -> github.com/vhqtvn/vh-agent-harness
//	https://github.com/vhqtvn/vh-agent-harness.git -> github.com/vhqtvn/vh-agent-harness
//	ssh://git@github.com/vhqtvn/vh-agent-harness  -> github.com/vhqtvn/vh-agent-harness
//	https://oauth2:TOKEN@github.com/vhqtvn/r.git  -> github.com/vhqtvn/r   (userinfo stripped)
//	https://gitlab.example.com/g/r.git/           -> gitlab.example.com/g/r (trailing slash + .git)
//
// Returns ("", false) for a local filesystem path remote or any string that is
// not a recognizable remote URL. Such a remote is skipped by RepoRemotes; a
// repo whose only remote is a local path is matched via the repo's absolute
// filesystem path instead.
func NormalizeRemote(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	// SCP-style: user@host:path
	if m := scpLikeRemoteRe.FindStringSubmatch(raw); m != nil {
		host := strings.ToLower(strings.TrimSpace(m[2]))
		p := cleanRepoPath(m[3])
		if host == "" || p == "" {
			return "", false
		}
		return host + "/" + p, true
	}

	// URL-style: scheme://[userinfo@]host/path
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		// Strip userinfo.
		if at := strings.Index(rest, "@"); at >= 0 {
			// Only treat as userinfo if the @ is before any '/'.
			if slash := strings.Index(rest, "/"); slash < 0 || at < slash {
				rest = rest[at+1:]
			}
		}
		// Split host / path at first '/'.
		slash := strings.Index(rest, "/")
		if slash < 0 {
			// scheme://host with no path is not a usable repo remote.
			return "", false
		}
		host := strings.ToLower(strings.TrimSpace(rest[:slash]))
		p := cleanRepoPath(rest[slash+1:])
		if host == "" || p == "" {
			return "", false
		}
		return host + "/" + p, true
	}

	// Anything else (a local path, a bare name) is not a normalizable remote.
	return "", false
}

// cleanRepoPath normalizes the path portion of a remote: it trims surrounding
// whitespace, strips ALL trailing slashes, strips a single trailing ".git",
// then strips trailing slashes again. This handles ".../repo.git",
// ".../repo.git/", and ".../repo/" uniformly. An empty result means there was
// no usable repo path.
func cleanRepoPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimRight(p, "/")
	p = strings.TrimSuffix(p, ".git")
	p = strings.TrimRight(p, "/")
	return p
}

// Binds reports whether this subject is ENFORCED for a repo identified by its
// absolute filesystem path and its normalized remotes.
//
// Binding rules:
//
//   - A subject with NO `repos:` list (nil or empty) binds ALL repos. This is
//     the machine-global default and the foundation of "protects every repo on
//     the machine with no per-repo setup".
//   - Otherwise the subject binds iff ANY `repos:` glob matches the repo's
//     absolute path OR any of its normalized remotes. Path globs cover
//     remoteless scratch repos; remote globs cover clones that move on disk.
//
// `repoPath` MUST be absolute; if it is not, Binds returns false (the caller is
// expected to pass an absolute path; non-absolute paths cannot be matched
// safely against path globs).
//
// This is the enforcement-scope predicate for BOTH subject kinds. The
// kind-specific ambient/source predicates are IsAmbient (forbidden-relation)
// and IsSource (scrub-project).
func (s Subject) Binds(repoPath string, remotes []string) bool {
	if len(s.Repos) == 0 {
		return true // machine-global
	}
	if repoPath != "" && matchAnyGlob(repoPath, s.Repos) {
		return true
	}
	for _, r := range remotes {
		if matchAnyGlob(r, s.Repos) {
			return true
		}
	}
	return false
}

// IsAmbient reports whether side A is AMBIENT for this repo, i.e. the repo's
// identity implies side A so only side B terms need to be matched. This is
// meaningful only for forbidden-relation subjects; for any other kind it
// returns false. The predicate matches the repo's path/remotes against
// `ambient_repos:`. An empty `ambient_repos:` list means side A is never
// ambient (both sides must co-occur to fire).
func (s Subject) IsAmbient(repoPath string, remotes []string) bool {
	if s.Kind != KindForbiddenRelation {
		return false
	}
	if len(s.AmbientRepos) == 0 {
		return false
	}
	if repoPath != "" && matchAnyGlob(repoPath, s.AmbientRepos) {
		return true
	}
	for _, r := range remotes {
		if matchAnyGlob(r, s.AmbientRepos) {
			return true
		}
	}
	return false
}

// IsSource reports whether this repo is itself a SENSITIVE ORIGIN for a
// scrub-project subject (i.e. matches `source_repos:`). Meaningful only for
// scrub-project subjects; for any other kind it returns false. An empty
// `source_repos:` list means the subject does not single out origin repos.
//
// RESERVED / CURRENTLY UNREACHABLE FROM LOADED REGISTRIES: validateSubject
// rejects any non-empty source_repos at load (v1 matches Labels only, mirroring
// the unit: diff precedent), so a Subject obtained from Load always has an empty
// SourceRepos and this predicate always returns false. The method and the
// SourceRepos field are retained for a future implementation that derives path
// fragments from source repo identities; until then this is dead code over
// loaded data. The predicate itself remains correct at the struct level (kept
// covered by TestSubject_IsSource).
func (s Subject) IsSource(repoPath string, remotes []string) bool {
	if s.Kind != KindScrubProject {
		return false
	}
	if len(s.SourceRepos) == 0 {
		return false
	}
	if repoPath != "" && matchAnyGlob(repoPath, s.SourceRepos) {
		return true
	}
	for _, r := range remotes {
		if matchAnyGlob(r, s.SourceRepos) {
			return true
		}
	}
	return false
}
