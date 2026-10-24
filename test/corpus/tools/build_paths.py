#!/usr/bin/env python3
"""Build test/corpus/paths.jsonl from clones of the repos in repos.json.

usage: build_paths.py CLONE_ROOT OUT.jsonl
CLONE_ROOT/<owner>_<repo> must be a checkout at the pinned commit (the script
refuses otherwise). Labels follow PATH-LABEL-RULES.txt; the repository's own
classifier is never imported or run. Deterministic: no RNG, sorted output.
"""
import hashlib, json, os, re, subprocess, sys
from collections import OrderedDict

DIR_CAP, CAND_CAP = 90, 40
HERE = os.path.dirname(os.path.abspath(__file__))
CAND = re.compile(r"(test|spec|secret|cred|token|key|env|cert|pem|kube|tf|rc$|id_|npmrc|netrc|gpg|keystore)", re.I)
SRC_EXT = {"go", "py", "js", "jsx", "ts", "tsx", "mjs", "cjs", "java", "kt", "swift", "dart", "rs", "c", "h", "rb", "php", "cs"}
JS_EXT = ("js", "jsx", "ts", "tsx", "mjs", "cjs", "mts", "cts")

def label(path):
    """Return (label, rule) or (None, reason) for an excluded path."""
    base = path.rsplit("/", 1)[-1]
    low = base.lower()
    ext0 = low.rsplit(".", 1)[-1] if "." in low else ""
    if low == "authorized_keys" or low.startswith(("id_rsa", "id_dsa", "id_ecdsa", "id_ed25519")) or low.endswith(".ppk"):
        return "ssh-key", "S1"
    if low in (".env", ".envrc") or low.startswith((".env.", ".env-")):
        return "env-file", "E1"
    if low in ("kubeconfig", ".boto", ".s3cfg", "application_default_credentials.json") or low.endswith((".tfstate", ".tfstate.backup", ".tfvars")):
        return "cloud-config", "C1"
    if low in (".netrc", "_netrc", ".pgpass", ".htpasswd", ".git-credentials", ".npmrc", ".pypirc", ".dockercfg") \
       or (re.fullmatch(r"(credentials|secrets?)(\..*)?", low) and ext0 not in SRC_EXT) \
       or low.endswith((".kdbx", ".keychain", ".gpg")):
        return "credential-shaped", "K1"
    ext = low.rsplit(".", 1)[-1] if "." in low else ""
    if ext in ("pem", "crt", "cer", "der", "p12", "pfx", "p7b", "jks", "keystore", "truststore", "key", "csr", "p8"):
        return "certificate", "X1"
    if base.endswith("_test.go"): return "test-file", "T-go"
    if base == "conftest.py" or (base.startswith("test_") and base.endswith(".py")) or base.endswith("_test.py"):
        return "test-file", "T-py"
    parts = low.split(".")
    if len(parts) >= 3 and parts[-1] in JS_EXT and parts[-2] in ("test", "spec", "test-d", "spec-d"):
        return "test-file", "T-js"
    if base.endswith(("Test.java", "Tests.java", "IT.java", "TestCase.java")): return "test-file", "T-java"
    if base.endswith(("Test.kt", "Tests.kt", "IT.kt")): return "test-file", "T-kt"
    if base.endswith("_test.dart"): return "test-file", "T-dart"
    if base.endswith(("Tests.swift", "Test.swift")): return "test-file", "T-swift"
    if path.endswith(".rs"):
        if re.search(r"(^|/)tests/[^/]+\.rs$", path) or re.search(r"(^|/)tests/[^/]+/main\.rs$", path):
            return "test-file", "T-rs-dir"
        if base == "tests.rs": return "test-file", "T-rs-name"
    if base.endswith((".java", ".kt")) and base.startswith("Test"):
        return None, "AMBIG-TestPrefix"
    if base.endswith(".swift") and "Test" in base and re.search(r"(^|/)Tests/", path):
        return None, "AMBIG-SwiftHelper"
    return "none", "N0"

def h(repo, p): return hashlib.sha256((repo + "\0" + p).encode()).hexdigest()

def main():
    root, out = sys.argv[1], sys.argv[2]
    repos = json.load(open(os.path.join(HERE, "..", "repos.json")))
    rows, excluded = [], {}
    for r in repos:
        d = os.path.join(root, r["id"].replace("/", "_"))
        head = subprocess.check_output(["git", "-C", d, "rev-parse", "HEAD"], text=True).strip()
        if head != r["commit_sha"]:
            sys.exit(f"{r['id']}: checkout at {head}, pinned {r['commit']}")
        raw = subprocess.check_output(["git", "-C", d, "ls-files", "-z"]).split(b"\0")
        paths = []
        for b in raw:
            if not b: continue
            try: p = b.decode("utf-8")
            except UnicodeDecodeError: excluded["bad-utf8"] = excluded.get("bad-utf8", 0) + 1; continue
            if "\t" in p or "\n" in p: excluded["tab-newline"] = excluded.get("tab-newline", 0) + 1; continue
            paths.append(p)
        groups = OrderedDict()
        for p in sorted(paths):
            comps = p.split("/")
            g = "/".join(comps[:2]) if len(comps) > 2 else (comps[0] if len(comps) == 2 else ".")
            groups.setdefault(g, []).append(p)
        for g in groups: groups[g].sort(key=lambda p: h(r["id"], p))
        picked, taken = [], set()
        names = sorted(groups); idx = {g: 0 for g in names}
        while len(picked) < DIR_CAP and any(idx[g] < len(groups[g]) for g in names):
            for g in names:
                if len(picked) >= DIR_CAP: break
                if idx[g] < len(groups[g]):
                    picked.append((groups[g][idx[g]], "dir:" + g)); idx[g] += 1
        taken = {p for p, _ in picked}
        cands = sorted((p for p in paths if p not in taken and CAND.search(p.rsplit("/", 1)[-1])), key=lambda p: h(r["id"], p))[:CAND_CAP]
        picked += [(p, "cand") for p in cands]
        taken |= set(cands)
        # Stratum "sens": every remaining file of a sensitive class, so the thin
        # classes (ssh-key, credential-shaped, cloud-config, env-file, certificate)
        # are covered whole rather than by sampling.
        picked += [(p, "sens") for p in sorted(paths) if p not in taken and label(p)[1] in ("S1", "E1", "C1", "K1", "X1")]
        for p, s in sorted(picked):
            lab, rule = label(p)
            if lab is None:
                excluded[rule] = excluded.get(rule, 0) + 1; continue
            rows.append({"repo": r["id"], "path": p, "label": lab, "rule": rule, "stratum": s.split(":")[0]})
    rows.sort(key=lambda x: (x["repo"], x["path"]))
    with open(out, "w") as f:
        for i, x in enumerate(rows, 1):
            f.write(json.dumps({"id": "p%04d" % i, **x}, ensure_ascii=False, separators=(",", ":")) + "\n")
    print("rows", len(rows), "excluded", json.dumps(excluded, sort_keys=True))

if __name__ == "__main__": main()
