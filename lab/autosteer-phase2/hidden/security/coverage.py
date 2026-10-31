"""Item 4 / R11 coverage: which CPython 2.7.18 advisories an arm names in any committed file at a commit.

Expected set: NVD's CPython-vulnerable entries for 2.7.18 (nvd_cpython.json, by cpeName) united with OSV's
advisories for the v2.7.18 commit (osv-v2.7.18.json, saved from the osv_cpython.sh query). Aliases are merged
first: an advisory counts as named if any of its ids (CVE, PSF, BIT) appears (C21, C33; E14 was this gap).
Blind spots: NVD by cpeName only; OSV maps few advisories to CPython commits (a lower bound, see osv_cpython.sh).

Usage: python3 coverage.py <arm> <ref>        Control: python3 coverage.py control
"""
import json
import os
import re
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ID = re.compile(r"\b(?:CVE-\d{4}-\d{4,}|PSF-\d{4}-\d+|BIT-[a-z-]+-\d{4}-\d+)\b")


def groups():
    """One set of ids per advisory, aliases merged; groups sharing any id are joined."""
    gs = [{c} for c in json.load(open(os.path.join(HERE, "nvd_cpython.json")))["2.7.18"]["cpython_vulnerable"]]
    gs += [{v["id"], *v.get("aliases", [])} for v in json.load(open(os.path.join(HERE, "osv-v2.7.18.json")))["vulns"]]
    merged = []
    for g in gs:
        for m in [m for m in merged if m & g]:
            merged.remove(m)
            g = g | m
        merged.append(g)
    return merged


def named_ids(repo, ref):
    files = subprocess.run(["git", "-C", repo, "ls-tree", "-r", "-z", "--name-only", ref], capture_output=True,
                           check=True).stdout.split(b"\0")
    ids = set()
    for f in filter(None, files):
        blob = subprocess.run(["git", "-C", repo, "show", b"%s:%s" % (ref.encode(), f)], capture_output=True).stdout
        ids.update(ID.findall(blob.decode("utf-8", "replace")))
    return ids


def coverage(arm, ref):
    repo = os.path.expanduser("~/rashomon-eval/py27/%s/py2scan" % arm)
    have = named_ids(repo, ref)
    gs = groups()
    missing = sorted(min(g) for g in gs if not g & have)
    return len(gs), len(gs) - len(missing), missing


def control():
    gs = groups()
    assert len(gs) == 37, len(gs)  # 35 NVD + 4 OSV, of which CVE-2023-27043 and its aliases are in both: 37 (L51)
    assert any({"PSF-2026-13", "CVE-2026-3479"} <= g for g in gs), "alias merge"
    assert ID.findall("see PSF-2026-14 and CVE-2007-4559.") == ["PSF-2026-14", "CVE-2007-4559"]
    n, named, _ = coverage("sonnet", "7517bf8")
    assert (n, named) == (37, 5), (n, named)  # L51 at 7517bf8: 32 of 37 named in no committed file
    print("coverage control ok: 37 advisories; S at 7517bf8 names 5 (L51)")


if __name__ == "__main__":
    if sys.argv[1:] == ["control"]:
        control()
    else:
        n, named, missing = coverage(sys.argv[1], sys.argv[2])
        print("%s %s: %d of %d CPython 2.7.18 advisories named in a committed file; not named: %d" % (
            sys.argv[1], sys.argv[2], named, n, len(missing)))
        print("  " + " ".join(missing))
