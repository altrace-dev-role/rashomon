"""Compare two hidden-check reports for one arm, metric by metric, both sides.

Usage: python3 compare_reports.py out/<arm>-<old>/report.json out/<arm>-<new>/report.json
Prints CHANGED or same for each metric, the new value, and the old value where it changed.
"""
import json
import sys


def summary(path):
    r = json.load(open(path))
    out = {"2b corpus identical, hostile cases differing": (r["item_2b_byte_identical_corpus"],
                                                           r["item_2b_hostile_cases_differing"])}
    for side in ("2.7", "3.14"):
        s = r["sides"][side]
        out[side + " builtin"] = s["H1_builtin"][1] if len(s["H1_builtin"]) > 1 else s["H1_builtin"]
        out[side + " false-ready by version"] = {k: v for k, v in s["H2_H8_false_ready_by_version"].items() if v}
        out[side + " false-ready examples"] = s["false_ready_examples"]
        h = s["H2b_H8s"]
        out[side + " R8 and except-comma"] = {k: h[k] for k in h if not isinstance(h[k], (list, dict))}
        out[side + " R8 examples"] = h.get("stdlib_examples")
        out[side + " hostile problems"] = {k: v["problems"] for k, v in s["H3_hostile"].items()
                                           if isinstance(v, dict) and v.get("problems")}
        out[side + " R6 unnamed"] = s["R6_files_not_named"]["count"]
        out[side + " determinism"] = s["determinism_jobs1_vs_default"]
        out[side + " 2c canary"] = s["item_2c_canary"]
        out[side + " corpus scan"] = {k: s["corpus_scan"][k] for k in ("files", "findings", "rc", "json_valid")}
    return out


if __name__ == "__main__":
    a, b = summary(sys.argv[1]), summary(sys.argv[2])
    for k in b:
        changed = a.get(k) != b[k]
        print("%s %s\n   new: %s" % ("CHANGED" if changed else "same   ", k, json.dumps(b[k])[:500]))
        if changed:
            print("   old: %s" % json.dumps(a.get(k))[:500])
