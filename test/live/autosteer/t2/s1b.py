"""s1b.py: T2 smoke check S-1b (addendum 8; the director's 11:43 PST ruling, item 1). Reads files only; no model call.

check ARM RUNDIR RECORDS_GLOB
    Reads RUNDIR/RECORDS_GLOB (keys.txt T2K_RECORDS) and names each declaration record's writes as the product names
    them. Prints one line and exits 0 when the arm's writes read in-project, 1 otherwise, the first that holds:
      no records       no file matches RECORDS_GLOB under RUNDIR
      paths withheld   a write record whose path, project or cwd is withheld (record scope v4), its own FAIL,
                       never read as 0 in-project
      path form split  a named write that strips against neither project nor cwd as strings, but sits under one of
                       them once symlinks resolve (t1c-r6: /tmp against /private/tmp, 28 of 28 Edit writes)
      0 in-project     no named write strips against project or cwd
table RECORDS_GLOB ROOT...
    One row per run directory (a directory holding an arm file) under each ROOT: the root as given, its name, arm,
    named writes, in-project, split, outside, withheld. No verdict.

The product's own count is TurnCounts.Writes (internal/verify/stop.go:278, countIn). At autosteer/m2 20a64198 no
record carries it: the Stop's StopView.Turn (stop.go:92) is not stored, and the stored checkpoint (type verify,
point stop) holds gate states only. So the rule below is a copy of internal/verify/classify.go at 20a64198:
writeTargets (:419-427) and appendWrites (:382-412) for the paths a declaration names; relativise (:449-464), under
(:484-490) and samePrefix (:494-499) for the strip; RelevantUnder's test (:475) for inside. Not copied: ClassOf,
Relevant and the gate roots (coveredByAny), so in-project here is wider than the counted writes. It is the
necessary half: a write that fails it is never counted.
"""
import glob
import json
import os
import sys

WITHHELD = ("file_path_withheld", "written_paths_withheld", "project_withheld", "cwd_withheld")


def slashed(s):
    return s.replace("\\", "/")


def same_prefix(a, b):
    if len(a) >= 2 and len(b) >= 2 and a[1] == ":" and b[1] == ":" and a[0].isalpha() and b[0].isalpha():
        return a[0].lower() == b[0].lower() and a[2:] == b[2:]
    return a == b


def under(p, d):
    d = d.rstrip("/")
    if d == "" or len(p) <= len(d) or p[len(d)] != "/" or not same_prefix(p[: len(d)], d):
        return p, False
    return p[len(d) + 1 :], True


def relativise(path, project, cwd):
    p = slashed(path)
    for d in (project, cwd):
        if d is None:
            continue
        rel, ok = under(p, slashed(d))
        if ok:
            p = rel
            break
    while p.startswith("./"):
        p = p[2:]
    return p


def inside(rel):
    return not (os.path.isabs(rel) or rel == ".." or rel.startswith("../"))


def real(s):
    return os.path.realpath(slashed(s))


def targets(d):
    """("none" | "named" | "blind", paths): writeTargets and appendWrites' switch."""
    shape = d.get("shape") or {}
    vc = shape.get("verb_class")
    if vc == "mcp":
        return "blind", []
    if d.get("tool_name") != "Bash" and vc == "write":
        fp = d.get("file_path")
        return ("named", [fp]) if fp is not None else ("blind", [])
    wp = d.get("written_paths") or []
    if wp:
        return "named", list(wp)
    if shape.get("may_write"):
        return "blind", []
    return "none", []


def read(rundir, records_glob):
    files = sorted(glob.glob(os.path.join(rundir, records_glob)))
    if not files:
        return None
    n = {"records": 0, "named": 0, "inproj": 0, "split": 0, "outside": 0, "withheld": 0, "unnamed": 0}
    projects, cwds, first_split = {}, {}, None
    for f in files:
        with open(f, errors="replace") as fh:
            for line in fh:
                try:
                    d = json.loads(line)
                except ValueError:
                    continue
                if not isinstance(d, dict) or d.get("type") != "declaration":
                    continue
                kind, paths = targets(d)
                if kind == "none":
                    continue
                n["records"] += 1
                if any(d.get(k) is not None for k in WITHHELD):
                    n["withheld"] += 1
                    continue
                if kind == "blind":
                    n["unnamed"] += 1
                    continue
                project, cwd = d.get("project"), d.get("cwd")
                projects[project] = projects.get(project, 0) + 1
                cwds[cwd] = cwds.get(cwd, 0) + 1
                for p in paths:
                    n["named"] += 1
                    if inside(relativise(p, project, cwd)):
                        n["inproj"] += 1
                    elif os.path.isabs(slashed(p)) and any(d0 is not None and under(real(p), real(d0))[1] for d0 in (project, cwd)):
                        n["split"] += 1
                        first_split = first_split or p
                    else:
                        n["outside"] += 1
    return n, projects, cwds, first_split


def top(m):
    if not m:
        return "none"
    k = max(m, key=lambda x: m[x])
    return "%s%s" % (k, "" if len(m) == 1 else " (%d distinct)" % len(m))


def check(arm, rundir, records_glob):
    r = read(rundir, records_glob)
    if r is None:
        print("S-1b %s: FAIL no records (%s under %s)" % (arm, records_glob, rundir))
        return 1
    n, projects, cwds, first_split = r
    counts = "write records %d, named writes %d, in-project %d, split %d, outside %d, withheld %d, unnamed %d" % (
        n["records"], n["named"], n["inproj"], n["split"], n["outside"], n["withheld"], n["unnamed"])
    where = "project %s; cwd %s" % (top(projects), top(cwds))
    if n["withheld"]:
        verdict = "FAIL paths withheld (%d write records; record scope v4 withholds what the check reads)" % n["withheld"]
    elif n["split"]:
        verdict = "FAIL path form split (%d of %d named writes strip against neither project nor cwd, yet sit under one once symlinks resolve; first %s)" % (
            n["split"], n["named"], first_split)
    elif n["inproj"] == 0:
        verdict = "FAIL 0 in-project writes"
    else:
        verdict = "pass"
    print("S-1b %s: %s; %s: %s" % (arm, counts, where, verdict))
    return 0 if verdict == "pass" else 1


def table(records_glob, roots):
    print("root\tname\tarm\tnamed\tin-project\tsplit\toutside\twithheld")
    for root in roots:
        for name in sorted(os.listdir(root)) if os.path.isdir(root) else []:
            d = os.path.join(root, name)
            if not os.path.isfile(os.path.join(d, "arm")):
                continue
            with open(os.path.join(d, "arm")) as fh:
                arm = fh.read().strip()
            r = read(d, records_glob)
            if r is None:
                print("%s\t%s\t%s\t-\t-\t-\t-\t-" % (root, name, arm))
            else:
                n = r[0]
                print("%s\t%s\t%s\t%d\t%d\t%d\t%d\t%d" % (root, name, arm, n["named"], n["inproj"], n["split"], n["outside"], n["withheld"]))
    return 0


def main(argv):
    if len(argv) == 4 and argv[0] == "check":
        return check(argv[1], argv[2], argv[3])
    if len(argv) >= 3 and argv[0] == "table":
        return table(argv[1], argv[2:])
    print("usage: s1b.py check ARM RUNDIR RECORDS_GLOB | table RECORDS_GLOB ROOT...", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
