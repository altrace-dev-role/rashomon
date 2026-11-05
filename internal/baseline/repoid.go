package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// RepoID is the repository's identity across machines and checkouts (HD11,
// PLAN B2): the normalized url (NormalizeOrigin) of the git remote named
// origin, read from the config of root's repository (Key) without running
// git (H-17). Without such a remote, or outside any repository, it is path:
// and the sha256 of the project key, and ephemeral is true: rows written
// under it are re-keyed when the repository gains an origin (an aliases
// entry, PLAN B2). The root-commit form the plan names for a repository
// with commits and no remote is not produced: the root commit sits behind
// packed objects this program does not read, and a form that changed when
// git packed them would be no identity. "" is "", ephemeral.
func RepoID(root string) (rid string, ephemeral bool) {
	if root == "" {
		return "", true
	}
	key := Key(root)
	if url, ok := originURL(key); ok {
		if n := NormalizeOrigin(url); n != "" {
			return n, false
		}
	}
	sum := sha256.Sum256([]byte(key))
	return "path:" + hex.EncodeToString(sum[:]), true
}

// GitDirs is the git directory of the repository at root and its common
// directory, found without running git (H-17): the .git directory itself,
// or for a worktree the directory its .git file points to (gitdir: path,
// relative to root or absolute) and the one that directory's commondir
// file names (relative to the git directory or absolute), the git
// directory again when there is no commondir file. HEAD is read from the
// git directory; config, the branch refs and packed-refs from the common
// one, which is where git keeps them for every worktree. false with no
// .git, or a .git file that is no pointer.
func GitDirs(root string) (gitDir, commonDir string, ok bool) {
	if root == "" {
		return "", "", false
	}
	dir := filepath.Join(root, ".git")
	st, err := os.Stat(dir)
	if err != nil {
		return "", "", false
	}
	if !st.IsDir() {
		b, err := os.ReadFile(dir)
		if err != nil {
			return "", "", false
		}
		rest, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
		if !ok {
			return "", "", false
		}
		dir = rest
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
	}
	common := dir
	if b, err := os.ReadFile(filepath.Join(dir, "commondir")); err == nil {
		common = strings.TrimSpace(string(b))
		if !filepath.IsAbs(common) {
			common = filepath.Join(dir, common)
		}
	}
	return dir, common, true
}

// originURL is the url of the remote named origin in the config of the
// repository at key (GitDirs' common directory); false with no repository,
// no config or no such remote. include and includeIf directives are not
// followed.
func originURL(key string) (string, bool) {
	_, common, ok := GitDirs(key)
	if !ok {
		return "", false
	}
	b, err := os.ReadFile(filepath.Join(common, "config"))
	if err != nil {
		return "", false
	}
	return configValue(string(b), `[remote "origin"]`, "url")
}

// configValue is key's last value under section in a git config text,
// false when the section or the key is absent. A section header matches
// with its inner spaces folded; a comment line starts with # or ;.
func configValue(text, section, key string) (string, bool) {
	in, val, found := false, "", false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			in = strings.Join(strings.Fields(line), " ") == section
			continue
		}
		if k, v, ok := strings.Cut(line, "="); in && ok && strings.TrimSpace(k) == key {
			val, found = strings.TrimSpace(v), true
		}
	}
	return val, found
}

// NormalizeOrigin is url as a repository id (PLAN B2): the scheme and any
// user name and password dropped, the host lowercased, an scp-like
// destination ([user@]host:path) read as host/path, a query, a fragment, a
// trailing slash and a trailing .git dropped, the path's case kept. A url
// with no host (file:///x, a local path) is that path. "" is "".
func NormalizeOrigin(url string) string {
	s := strings.TrimSpace(url)
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	var host, path string
	switch i := strings.Index(s, "://"); {
	case i >= 0:
		host, path, _ = strings.Cut(s[i+3:], "/")
	case strings.Contains(s, ":") && !strings.Contains(s[:strings.Index(s, ":")], "/"):
		host, path, _ = strings.Cut(s, ":")
	default:
		return trimRepo(s)
	}
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	host, path = strings.ToLower(host), trimRepo(path)
	switch {
	case host == "" && path == "":
		return ""
	case host == "":
		return "/" + path
	}
	return host + "/" + path
}

// trimRepo drops a trailing slash and a trailing .git from a repository path.
func trimRepo(p string) string {
	p = strings.TrimRight(p, "/")
	return strings.TrimRight(strings.TrimSuffix(p, ".git"), "/")
}
