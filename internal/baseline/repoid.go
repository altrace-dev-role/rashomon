package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// MaxGitFileBytes is the most this package reads of each git file it
// reads: the .git pointer file, commondir and config. It bounds a file git
// or a person writes, not a size measured from one. Git sets no limit on a
// config, and crossing the cap is a hard failure, so the cap sits far above
// a working file. A file over it is refused, never read in part: a config
// cut short could lose the origin's line and give another id.
const MaxGitFileBytes = 1 << 20

// ErrNotRepository is GitDirsErr's error when root is no repository: no
// .git, or a .git file that is no gitdir: pointer. RepoID's path form and
// headCommit's C1 are for it alone, never for a refused read.
var ErrNotRepository = errors.New("not a git repository")

// RepoID is the repository's identity across machines and checkouts (HD11,
// PLAN B2): the normalized url (NormalizeOrigin) of the git remote named
// origin, read from the config of root's repository (Key) without running
// git (H-17). Without such a remote, with one whose url NormalizeOrigin
// refuses (an @ after its host part), or outside any repository, it is path:
// and the sha256 of the project key, and ephemeral is true: rows written
// under it are re-keyed when the repository gains an origin (an aliases
// entry, PLAN B2). The root-commit form the plan names for a repository
// with commits and no remote is not produced: the root commit sits behind
// packed objects this program does not read, and a form that changed when
// git packed them would be no identity. "" is "", ephemeral. A git file
// that is there and refused (RepoIDErr's error: not a regular file, over
// MaxGitFileBytes, or a read that failed) is "", ephemeral too: no id,
// never the path form, which would key the repository's rows apart from
// its origin's. The hook path reads "" as nothing to capture.
func RepoID(root string) (rid string, ephemeral bool) {
	rid, ephemeral, err := RepoIDErr(root)
	if err != nil {
		return "", true
	}
	return rid, ephemeral
}

// RepoIDErr is RepoID with a refused git read as its error, naming the file
// (GitDirsErr, and the config read through the same cap), for a caller off
// the hook path that fails loud rather than key anything under no id: graph
// index and graph knowledge import.
func RepoIDErr(root string) (rid string, ephemeral bool, err error) {
	if root == "" {
		return "", true, nil
	}
	key := Key(root)
	url, ok, err := originURL(key)
	if err != nil {
		return "", true, err
	}
	if ok {
		if n := NormalizeOrigin(url); n != "" {
			return n, false, nil
		}
	}
	sum := sha256.Sum256([]byte(key))
	return "path:" + hex.EncodeToString(sum[:]), true, nil
}

// GitDirs is the git directory of the repository at root and its common
// directory, found without running git (H-17): the .git directory itself,
// or for a worktree the directory its .git file points to (gitdir: path,
// relative to root or absolute) and the one that directory's commondir
// file names (relative to the git directory or absolute), the git
// directory again when there is no commondir file. HEAD is read from the
// git directory; config, the branch refs and packed-refs from the common
// one, which is where git keeps them for every worktree. false with no
// .git, a .git file that is no pointer, or a refused read (GitDirsErr).
func GitDirs(root string) (gitDir, commonDir string, ok bool) {
	gitDir, commonDir, err := GitDirsErr(root)
	return gitDir, commonDir, err == nil
}

// GitDirsErr is GitDirs with its reason. ErrNotRepository is no .git, or a
// .git file that is no pointer: GitDirs' false before reads were refused.
// Any other error is a refusal naming the file: a .git that cannot be
// stat'd, or a pointer or commondir file that is not a regular file (a
// named pipe is refused at once, never waited on), is over MaxGitFileBytes,
// or cannot be read. Only a commondir that is absent (absent: not found,
// or a path through a regular file) gives the git directory as the common
// one. A refused commondir is never that fall back, which would read the
// worktree's own config, another file.
func GitDirsErr(root string) (gitDir, commonDir string, err error) {
	if root == "" {
		return "", "", ErrNotRepository
	}
	dir := filepath.Join(root, ".git")
	st, err := os.Stat(dir)
	switch {
	case absent(err):
		return "", "", ErrNotRepository
	case err != nil:
		return "", "", err
	}
	if !st.IsDir() {
		b, err := readGitFile(dir)
		if err != nil {
			return "", "", err
		}
		rest, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
		if !ok {
			return "", "", ErrNotRepository
		}
		dir = rest
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
	}
	common := dir
	switch b, err := readGitFile(filepath.Join(dir, "commondir")); {
	case err == nil:
		common = strings.TrimSpace(string(b))
		if !filepath.IsAbs(common) {
			common = filepath.Join(dir, common)
		}
	case !absent(err):
		return "", "", err
	}
	return dir, common, nil
}

// absent reports an error that says the path names no file: not found, or
// (ENOTDIR, on unix) a path through a regular file, which windows reports
// as not found. A pointer to a missing or plain-file git directory is so no
// repository git would read, and RepoID gives it the path form, as before
// reads were refused (TestRepoID_APathThroughAFileIsAbsent).
func absent(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// readGitFile is the file at p, opened through regfile.Open (a named pipe,
// a socket, a device or a directory is refused at once, never waited on)
// and read up to MaxGitFileBytes; a file over the cap is an error naming it
// and the cap, never read in part. A missing file is the open's own error.
func readGitFile(p string) ([]byte, error) {
	f, err := regfile.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxGitFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxGitFileBytes {
		return nil, fmt.Errorf("%s: over %d bytes", p, MaxGitFileBytes)
	}
	return b, nil
}

// originURL is the url of the remote named origin in the config of the
// repository at key (GitDirsErr's common directory); false with no
// repository, no config or no such remote; an error for a refused read of
// the pointer, commondir or config (readGitFile). include and includeIf
// directives are not followed.
func originURL(key string) (string, bool, error) {
	_, common, err := GitDirsErr(key)
	switch {
	case errors.Is(err, ErrNotRepository):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	b, err := readGitFile(filepath.Join(common, "config"))
	switch {
	case absent(err):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	url, ok := configValue(string(b), `[remote "origin"]`, "url")
	return url, ok, nil
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
// with no host (file:///x, a local path) is that path. "" is "", and so is
// a url with an @ after its host part, looked for in the whole url before
// the query is cut: after the first /, ? or # past a scheme's ://, after
// the first colon of an scp-like destination, anywhere in a url of neither
// form. Such an @ may end a user name or password holding that delimiter,
// which would otherwise be read as the host or the path. So the result
// never holds an @, and in the two host forms no byte from before the url's
// last @ (TestNormalizeOrigin_KeepsNoUserPart).
func NormalizeOrigin(url string) string {
	whole := strings.TrimSpace(url)
	s := whole
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	var host, path string
	switch i := strings.Index(s, "://"); {
	case i >= 0:
		if end := strings.IndexAny(whole[i+3:], "/?#"); end >= 0 && strings.Contains(whole[i+3+end:], "@") {
			return ""
		}
		host, path, _ = strings.Cut(s[i+3:], "/")
	case strings.Contains(s, ":") && !strings.Contains(s[:strings.Index(s, ":")], "/"):
		if strings.Contains(whole[strings.Index(s, ":")+1:], "@") {
			return ""
		}
		host, path, _ = strings.Cut(s, ":")
	default:
		if strings.Contains(whole, "@") {
			return ""
		}
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
