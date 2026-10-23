"""Scores each arm on the second opinion's 43 hidden-class cases against per-version oracles (2.7.18, 3.6-3.15).
Classes scored: crash (traceback, invalid JSON, exit outside 0-2, timeout); R5 (an unlistable directory needs exit 2 and
a listing; a non-regular entry is skipped only if listed); guarantee (a .py file a judged version cannot compile gets a
blocker for that version); precision (spec-defined constructs inside strings get no finding). Judgment twins unscored.
Item 2b (added 02:08): every case's JSON and exit code must be identical under 2.7 and 3.14; judgment twins included,
since 2b holds for any input. A case with a traceback on either side is reported under crash, not here."""
import json, glob, re, sys
J = ["3.6", "3.7", "3.8", "3.9", "3.10", "3.11", "3.12", "3.13", "3.14", "3.15"]
OR = {v: json.load(open("oracle-%s.json" % v))["cases"] for v in ["2.7.18"] + J}
PRECISION = {"ne_in_string", "backtick_in_string", "long_in_string", "except_comma_in_string"}
JUDGMENT = {"print_tuple", "print_single", "exec_call_form", "has_key", "future_division", "octal_zero", "br_prefix",
            "formfeed_indent", "mixed_newlines", "tabs_only", "only_shebang", "hardlink_pair", "case_pair", "many_files",
            "pyc_only", "utf8_sig_cookie", "nesting_20"}

def strings(o):
    if isinstance(o, dict):
        for k, v in o.items():
            yield str(k); yield from strings(v)
    elif isinstance(o, list):
        for v in o: yield from strings(v)
    elif isinstance(o, str):
        yield o

def named(j, name):
    toks = set()
    for s in strings(j):
        toks.add(s); toks.update(t.strip(".:") for t in re.split(r"[\s\"'()\[\]{},;]+", s))
    return name in toks or any(t.endswith("/" + name) for t in toks)

def score(arm, side):
    res = json.load(open("arm-%s-%s.json" % (arm, side)))
    probs = {}
    for case, r in sorted(res.items()):
        p = []
        j = r["json"]
        if r["rc"] == 124: p.append("timeout")
        elif r["tb"]: p.append("traceback: %s" % (r["err"][0][:60] if r["err"] else "?"))
        elif j is None: p.append("no valid JSON (exit %d)" % r["rc"])
        elif r["rc"] not in (0, 1, 2): p.append("exit %d" % r["rc"])
        if j is not None and not p:
            o27 = OR["2.7.18"].get(case, {})
            ents = o27.get("entries", {})
            if case == "unlistable_py":
                if r["rc"] != 2: p.append("R5: exit %d, want 2" % r["rc"])
                if not named(j, "pkg.py"): p.append("R5: pkg.py not listed")
            for n, kind in ents.items():
                if kind in ("socket", "symlink", "other", "fifo") and n.endswith(".py") or (kind == "symlink"):
                    if case in ("symlink_chain",) and n != "a1.py": continue
                    # skipped entries must be listed; scanned ones are named anyway
                    if not named(j, n): p.append("R5: %s (%s) not listed" % (n, kind))
            if case in PRECISION:
                b = [f for f in j.get("findings", []) if f.get("file", "").endswith("t.py") and f.get("severity") == "blocker"]
                if b: p.append("precision: %s on a string" % b[0].get("rule"))
            elif case not in JUDGMENT:
                for fname, verdict in OR["3.14"].get(case, {}).get("compile", {}).items():
                    if not fname.endswith(".py"): continue  # many_files and very_deep_dirs carry counts, not names
                    fail_v = [v for v in J if OR[v].get(case, {}).get("compile", {}).get(fname, "ok") != "ok"]
                    if not fail_v: continue
                    have = set()
                    for f in j.get("findings", []):
                        if f.get("file", "").split("/")[-1] == fname and f.get("severity") == "blocker":
                            have.update(f.get("versions") or J)
                    miss = [v for v in fail_v if v not in have]
                    if miss: p.append("guarantee: %s free of blockers for %s (no 3.x of those compiles it)" % (fname, ",".join(miss) if len(miss) < 4 else miss[0] + ".." + miss[-1]))
        if p: probs[case] = p
    return probs

def flat(o, p=""):
    if isinstance(o, dict):
        for k, v in o.items(): yield from flat(v, "%s/%s" % (p, k))
    elif isinstance(o, list):
        for i, v in enumerate(o): yield from flat(v, "%s[%d]" % (p, i))
    else:
        yield p, o

def two_b(a, b):
    """Cases whose JSON or exit code differs between the sides: {case: 'message text only' | 'findings or readiness'}."""
    out = {}
    for c in sorted(set(a) | set(b)):
        ra, rb = a.get(c), b.get(c)
        if not ra or not rb or ra["tb"] or rb["tb"]: continue
        if (ra["json"], ra["rc"]) == (rb["json"], rb["rc"]): continue
        fa, fb = dict(flat(ra["json"])), dict(flat(rb["json"]))
        paths = [q for q in set(fa) | set(fb) if fa.get(q) != fb.get(q)]
        out[c] = "message text only" if ra["rc"] == rb["rc"] and paths and all(q.endswith("/message") for q in paths) \
            else "findings or readiness"
    return out

def two_b_control():
    a = {"x": {"tb": False, "rc": 1, "json": {"findings": [{"message": "'utf8' codec", "rule": "R"}]}},
         "y": {"tb": False, "rc": 0, "json": {"findings": []}}, "z": {"tb": True, "rc": 1, "json": None}}
    b = json.loads(json.dumps(a))
    assert two_b(a, b) == {}
    b["x"]["json"]["findings"][0]["message"] = "'utf-8' codec"
    b["y"]["rc"] = 1
    assert two_b(a, b) == {"x": "message text only", "y": "findings or readiness"}, two_b(a, b)

if __name__ == "__main__":
    two_b_control()
    tot = {}
    for arm in ("opus", "sonnet", "blank"):
        head = open("arm-%s.head" % arm).read().strip()
        for side in ("2.7", "3.14"):
            pr = score(arm, side)
            tot[(arm, side)] = pr
            print("== %s %s %s: %d of 43 cases with a problem" % (arm, head, side, len(pr)))
            for c, p in pr.items(): print("   %-24s %s" % (c, "; ".join(p)[:150]))
    print("\n== item 2b on the battery: cases whose JSON or exit code differs between 2.7 and 3.14 (control ok)")
    for arm in ("opus", "sonnet", "blank"):
        head = open("arm-%s.head" % arm).read().strip()
        d = two_b(*[json.load(open("arm-%s-%s.json" % (arm, side))) for side in ("2.7", "3.14")])
        print("   %s %s: %d (%s)" % (arm, head, len(d), "; ".join("%s: %s" % kv for kv in sorted(d.items())) or "-"))
