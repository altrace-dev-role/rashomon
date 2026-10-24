#!/usr/bin/env python3
"""Extract and label shell lines from the pinned clones (see SHELL-LABEL-RULES.txt).

usage: build_shell.py CLONE_ROOT OUT_CANDIDATES.jsonl
Writes UNSCRUBBED candidates; cmd/scrubfilter turns them into shell.jsonl.
The repository's own classifier is never imported or run. Deterministic.
"""
import hashlib, json, os, re, shlex, subprocess, sys
import yaml

HERE = os.path.dirname(os.path.abspath(__file__))
CAPS = {"workflow": 60, "make": 30, "script": 40, "readme": 30}
MAXLEN = 400
STRUCT = {"if", "then", "else", "elif", "fi", "for", "while", "until", "do", "done", "case", "esac", "function", "select"}
ASSIGN = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*=")
OPS = {"&&", "||", ";", "|", "&", ";;", "|&"}
READ = set("base64 cat less more head tail ls ll find grep egrep fgrep rg ag wc stat file tree diff cmp realpath readlink pwd sort uniq cut awk jq yq sha256sum shasum md5sum which whoami uname id hostname date df du ps printenv basename dirname nproc".split())
WRITE = set("install rm mv cp mkdir rmdir touch tee ln chmod chown chgrp truncate dd patch mktemp unzip zip gzip gunzip bzip2 xz unxz 7z".split())
NET = set("curl wget ssh scp sftp rsync nc ncat dig nslookup host ping telnet ftp aws gcloud az".split())
VCS = {"git", "gh", "hg", "svn"}
PKG = set("npm npx pnpm pnpx yarn bun bunx pip pip3 pipx uv poetry conda gem bundle brew apt apt-get apk dnf yum choco winget go cargo rustup swift dart flutter dotnet gradle gradlew mvn mvnw pod xcodebuild".split())
RUNNERS = set("pytest py.test jest vitest mocha rspec phpunit ctest karma".split())
NONTEST_ENV = re.compile(r"lint|docs|mypy|type|fmt|format|style|build|coverage-report|pre-commit", re.I)
WRAPPER_SKIP = {"sudo": 0, "time": 0, "nice": 0, "nohup": 0, "command": 0, "exec": 0, "env": 0, "timeout": 1}

class Skip(Exception): pass
class Ambig(Exception): pass

def tokens(line):
    lx = shlex.shlex(line, posix=True, punctuation_chars=True)
    lx.whitespace_split = True
    lx.commenters = ""
    try: return list(lx)
    except ValueError: raise Skip("lex")

def first_simple(toks):
    """Skip leading cd steps and assignments; return the argv of the leading command."""
    i = 0
    while True:
        j = i
        while j < len(toks) and toks[j] not in OPS: j += 1
        cmd = toks[i:j]
        k = 0
        while k < len(cmd) and ASSIGN.match(cmd[k]): k += 1
        cmd = cmd[k:]
        if cmd and cmd[0] == "cd" and j < len(toks) and toks[j] in ("&&", ";"):
            i = j + 1
            continue
        return cmd

def unwrap(cmd):
    while cmd and cmd[0] in WRAPPER_SKIP:
        w = cmd[0]; cmd = cmd[1:]
        if w == "env":
            while cmd and (cmd[0].startswith("-") or ASSIGN.match(cmd[0])): cmd = cmd[1:]
        else:
            while cmd and cmd[0].startswith("-"): cmd = cmd[1:]
            for _ in range(WRAPPER_SKIP[w]):
                if cmd: cmd = cmd[1:]
        k = 0
        while k < len(cmd) and ASSIGN.match(cmd[k]): k += 1
        cmd = cmd[k:]
    return cmd

def nonflags(args, valued=()):
    out, skip = [], False
    for a in args:
        if skip: skip = False; continue
        if a == "--": break
        if a in valued: skip = True; continue
        if a.startswith("-"): continue
        out.append(a)
    return out

def js_pm(args):
    seq = nonflags(args, valued=("--filter", "-F", "--prefix", "-C", "--cwd", "-w", "--workspace"))
    for n, a in enumerate(seq):
        if a in ("test", "t", "tst"): return "test"
        if a in ("run", "run-script"):
            nxt = seq[n + 1] if n + 1 < len(seq) else ""
            return "test" if nxt.startswith("test") else "package"
        if a in ("install", "i", "ci", "add", "remove", "build", "lint", "publish", "pack", "exec", "dlx", "update", "up", "audit", "init", "version", "link", "dedupe", "prune", "outdated"): return "package"
    return "package"

def classify_cmd(cmd, depth=0):
    """Return (class, rule); raise Ambig/Skip."""
    cmd = unwrap(cmd)
    if not cmd: raise Skip("empty")
    p0 = cmd[0]
    if "$" in p0 or "`" in p0 or p0 in ("(", ")", "{", "}", "!", "<", ">", ">>", "2>"): raise Skip("dynamic-program")
    if not re.fullmatch(r"[A-Za-z0-9_./~+@-]+|\[\[?|\.", p0): raise Skip("not-a-program-word")
    p = os.path.basename(p0)
    a = cmd[1:]
    if p in VCS: return "vcs", "vcs"
    if p in NET: return "network", "net"
    if p == "sed" or p == "perl":
        inplace = any(re.match(r"^-[a-zA-Z]*i", x) or x == "--in-place" for x in a if x.startswith("-") and not x.startswith("--") or x == "--in-place")
        return ("write", "write-inplace") if inplace else ("read", "read-" + p)
    if p == "find":
        if "-delete" in a: return "write", "write-find-delete"
        if any(x in a for x in ("-exec", "-execdir", "-ok", "-fprint")): raise Ambig("find-exec")
        return "read", "read"
    if p == "tar":
        letters = (a[0].lstrip("-") if a else "")
        if "x" in letters or "c" in letters: return "write", "write-tar"
        if "t" in letters: return "read", "read-tar"
        raise Ambig("tar")
    if p in RUNNERS:
        return "test", "test-runner"
    if p in ("tox", "nox"):
        return ("execute", "exec-tox-nontest") if NONTEST_ENV.search(" ".join(a)) else ("test", "test-tox")
    if p in ("python", "python3", "py"):
        if len(a) >= 2 and a[0] == "-m":
            if a[1] in ("pytest", "unittest"): return "test", "test-python-m"
            if a[1] in ("pip", "build", "pipx", "ensurepip"): return "package", "pkg-python-m"
        return "execute", "exec"
    if p == "make" or p == "gmake":
        s = nonflags(a, valued=("-C", "-f", "-j", "-I", "-o", "-W", "-l"))
        t = s[0] if s else "all"
        if t in ("test", "tests") or t.startswith("test-") or t.startswith("test_"): return "test", "test-make"
        if t in ("check", "verify", "ci", "all", "precommit"): raise Ambig("make-" + t)
        return "execute", "exec-make"
    if p == "rake":
        s = nonflags(a)
        return ("test", "test-rake") if s and s[0] in ("test", "spec") else ("execute", "exec")
    if p in ("gradle", "gradlew"):
        tasks = nonflags(a, valued=("-x", "--exclude-task", "-p", "--project-dir", "-c", "--settings-file", "-b", "--build-file", "-P", "--init-script", "-I"))
        tasks = [t for t in tasks if not t.startswith("-D")]
        low = [t.split(":")[-1].lower() for t in tasks]
        if any(t.startswith("test") or t.endswith("test") or t.endswith("tests") for t in low): return "test", "test-gradle"
        if any(t in ("check", "verify", "ci") for t in low): raise Ambig("gradle-check")
        return "package", "pkg"
    if p in ("mvn", "mvnw"):
        goals = nonflags(a, valued=("-f", "-pl", "-P", "-s", "-T", "-rf", "--file", "--projects", "-D"))
        goals = [g for g in goals if not g.startswith("-D")]
        if any(g in ("test", "integration-test", "surefire:test", "failsafe:integration-test") for g in goals): return "test", "test-mvn"
        if any(g in ("verify", "install") for g in goals) and "verify" in goals: raise Ambig("mvn-verify")
        return "package", "pkg"
    if p == "xcodebuild":
        return ("test", "test-xcodebuild") if any(x in ("test", "test-without-building") for x in a) else ("package", "pkg")
    if p in ("npm", "pnpm", "yarn", "bun"):
        c = js_pm(a)
        return c, ("test-js" if c == "test" else "pkg")
    if p in ("npx", "pnpx", "bunx", "uv", "poetry", "pipx", "bundle"):
        inner = None
        if p in ("npx", "pnpx", "bunx"):
            s = nonflags(a, valued=("-p", "--package", "-c", "--call"))
            inner = s
        elif p in ("uv", "poetry") and a and a[0] == "run":
            rest, k = a[1:], 0
            while k < len(rest) and rest[k].startswith("-"):
                k += 2 if rest[k] in ("--with", "--python", "-p", "--extra", "--group", "--project", "--directory", "--env-file", "--with-requirements") else 1
            inner = rest[k:]
        elif p == "pipx" and a and a[0] == "run":
            inner = nonflags(a[1:])
        elif p == "bundle" and a and a[0] == "exec":
            inner = a[1:]
        if inner and depth < 2:
            try:
                c, _ = classify_cmd(inner, depth + 1)
            except (Skip, Ambig):
                return "package", "pkg-wrapper"
            if c == "test": return "test", "test-wrapper"
        return "package", "pkg"
    if p == "go":
        s = nonflags(a)
        return ("test", "test-go") if s and s[0] == "test" else ("package", "pkg-go")
    if p == "cargo":
        s = [x for x in a if not x.startswith("+")]
        s = nonflags(s)
        return ("test", "test-cargo") if s and (s[0] == "test" or (s[0] == "nextest")) else ("package", "pkg-cargo")
    if p in ("swift", "dart", "flutter", "dotnet"):
        s = nonflags(a)
        return ("test", "test-" + p) if s and s[0] == "test" else ("package", "pkg")
    if p in PKG: return "package", "pkg"
    if p in READ: return "read", "read"
    if p in WRITE: return "write", "write"
    return "execute", "exec" if p in EXEC_NAMED else "exec-default"

EXEC_NAMED = set("echo printf cd export set source . true false exit sleep [ [[ test trap unset eval read bash sh zsh python python3 node ruby java gcc cc g++ clang rustc javac tsc eslint prettier ruff black mypy golangci-lint shellcheck pre-commit docker podman kubectl helm terraform xargs cmake ninja".split())

PLACEHOLDER = re.compile(r"<[A-Za-z][A-Za-z0-9_-]*>|\.\.\.|\u2026")
MAKEVAR = re.compile(r"\$[@<^?*%]")

def label_line(line, kind=""):
    if kind == "readme" and PLACEHOLDER.search(line): raise Skip("placeholder")
    if kind == "make" and MAKEVAR.search(line): raise Skip("make-automatic-variable")
    toks = tokens(line)
    if not toks: raise Skip("empty")
    if toks[0] in STRUCT or toks[0] in ("}", "{", "(") or line.rstrip().endswith("{") or line.lstrip().startswith("("): raise Skip("structure")
    cmd = first_simple(toks)
    if not cmd: raise Skip("assign-only")
    return classify_cmd(cmd)

def logical_lines(text):
    """Yield logical shell lines: continuations joined, comments/blank/heredoc bodies dropped."""
    raw = text.split("\n"); i = 0; heredoc = None
    while i < len(raw):
        ln = raw[i]; i += 1
        if heredoc is not None:
            if ln.strip() == heredoc: heredoc = None
            continue
        s = ln.strip()
        if not s or s.startswith("#"): continue
        while s.endswith("\\") and i < len(raw):
            s = s[:-1].rstrip() + " " + raw[i].strip(); i += 1
        # unbalanced quotes: pull in following lines (bounded)
        n = 0
        while n < 20 and i < len(raw) and not balanced(s):
            s += "\n" + raw[i]; i += 1; n += 1
        m = re.search(r"<<-?\s*['\"]?([A-Za-z_][A-Za-z0-9_]*)['\"]?", s)
        if m: heredoc = m.group(1)
        yield s

def balanced(s):
    try:
        lx = shlex.shlex(s, posix=True); lx.whitespace_split = True; lx.commenters = ""
        list(lx); return True
    except ValueError: return False

def h(*parts): return hashlib.sha256("\0".join(parts).encode()).hexdigest()

def workflow_runs(path):
    try: doc = yaml.safe_load(open(path, encoding="utf-8"))
    except Exception: return
    jobs = []
    if isinstance(doc, dict) and isinstance(doc.get("jobs"), dict):
        for j in doc["jobs"].values():
            if isinstance(j, dict): jobs.append((str(j.get("runs-on", "")), (j.get("defaults") or {}).get("run", {}) or {}, j.get("steps") or []))
    elif isinstance(doc, dict) and isinstance(doc.get("runs"), dict):
        jobs.append(("", {}, doc["runs"].get("steps") or []))
    for runs_on, dflt, steps in jobs:
        for st in steps:
            if not isinstance(st, dict) or not isinstance(st.get("run"), str): continue
            sh = str(st.get("shell") or dflt.get("shell") or "")
            if sh and sh.split()[0] in ("pwsh", "powershell", "cmd", "python"): continue
            if not sh and "windows" in runs_on.lower(): continue
            yield st["run"]

def make_lines(text):
    for ln in text.split("\n"): pass
    raw = text.split("\n"); i = 0
    while i < len(raw):
        ln = raw[i]; i += 1
        if not ln.startswith("\t"): continue
        s = ln.strip()
        while s.endswith("\\") and i < len(raw): s = s[:-1].rstrip() + " " + raw[i].strip(); i += 1
        s = s.lstrip("@-+ ")
        if s and not s.startswith("#") and "$(" not in s: yield s

FENCE = re.compile(r"^(\s*)(```|~~~)\s*([A-Za-z0-9_+-]*)")
def readme_blocks(text):
    raw = text.split("\n"); i = 0
    while i < len(raw):
        m = FENCE.match(raw[i]); i += 1
        if not m: continue
        lang = m.group(3).lower(); body = []
        while i < len(raw) and not raw[i].lstrip().startswith(m.group(2)): body.append(raw[i].strip()); i += 1
        i += 1
        if lang in ("sh", "bash", "shell", "zsh"): yield "\n".join(body)
        elif lang == "console": yield "\n".join(l[2:] for l in body if l.startswith("$ "))

def main():
    root, out = sys.argv[1], sys.argv[2]
    repos = json.load(open(os.path.join(HERE, "..", "repos.json")))
    seen, log = {}, {"skip": {}, "ambig": {}, "overlong": 0, "too-many-skipped-nonshell": 0}
    cands = []
    for r in repos:
        d = os.path.join(root, r["id"].replace("/", "_"))
        head = subprocess.check_output(["git", "-C", d, "rev-parse", "HEAD"], text=True).strip()
        if head != r["commit_sha"]: sys.exit(f"{r['id']}: checkout at {head}, pinned {r['commit']}")
        files = [f for f in subprocess.check_output(["git", "-C", d, "ls-files", "-z"]).decode("utf-8", "replace").split("\0") if f]
        per = {k: [] for k in CAPS}
        for f in sorted(files):
            low = f.lower(); full = os.path.join(d, f)
            if not os.path.isfile(full) or os.path.islink(full): continue
            kind = None
            if re.match(r"\.github/(workflows/[^/]+\.ya?ml|actions/.+/action\.ya?ml)$", low): kind = "workflow"
            elif re.search(r"(^|/)(gnumakefile|makefile|[^/]+\.mk)$", low): kind = "make"
            elif low.endswith((".sh", ".bash")): kind = "script"
            elif low.endswith(".md"): kind = "readme"
            if not kind: continue
            try: text = open(full, encoding="utf-8").read()
            except Exception: continue
            if kind == "workflow": blobs = list(workflow_runs(full))
            elif kind == "make": blobs = ["\n".join(make_lines(text))]
            elif kind == "script": blobs = [text]
            else: blobs = list(readme_blocks(text))
            for b in blobs:
                for s in logical_lines(b):
                    if len(s.encode()) > MAXLEN: log["overlong"] += 1; continue
                    try:
                        cls, rule = label_line(s, kind)
                        if kind == "readme" and rule == "exec-default": raise Skip("readme-unnamed-program")
                    except Skip as e: log["skip"][str(e)] = log["skip"].get(str(e), 0) + 1; continue
                    except Ambig as e: log["ambig"][str(e)] = log["ambig"].get(str(e), 0) + 1; continue
                    per[kind].append((h(r["id"], kind, f, s), r["id"], kind, f, s, cls, rule))
        for kind, rows in per.items():
            uniq, ks = [], set()
            for x in sorted(rows):
                if x[4] in ks: continue
                ks.add(x[4]); uniq.append(x)
            for x in uniq[:CAPS[kind]]: cands.append(x)
    # global merge of identical lines
    merged = {}
    for _, repo, kind, f, s, cls, rule in cands:
        if s in merged: merged[s]["count"] += 1; continue
        merged[s] = {"repo": repo, "source": kind, "file": f, "line": s, "label": cls, "rule": rule, "count": 1}
    rows = sorted(merged.values(), key=lambda x: (x["repo"], x["source"], x["file"], x["line"]))
    with open(out, "w") as fh:
        for n, x in enumerate(rows, 1):
            fh.write(json.dumps({"id": "s%04d" % n, **x}, ensure_ascii=False, separators=(",", ":")) + "\n")
    print("candidates", len(rows), "log", json.dumps(log, sort_keys=True))

if __name__ == "__main__": main()
