"""truth_read.py: the blind second-read agreement judge for the AC-M1.1 and AC-M1.5 truth sets (W-M1.19).

Input: a JSON list of panel rows, one per prompt id: {"id", "cls", "reads": [<unit or null>, ...]}, where each
read is one blind reader's answer (an import path or "-"), and the truth files (prompts.tsv, edge.tsv,
classes.tsv: id, unit, ...). A truth is ';'-separated alternatives; '-' means no unit owns the work.

Per row: the readers' majority (more than half of the reads on one answer), and a verdict: confirmed (the
majority is one of the truth's alternatives), contested (a majority names another unit) or split (no
majority). Per class: counts of each verdict, unanimous rows, and reader agreement with the truth over all
reads. Fleiss' kappa over the readers says how far the readers agree with each other beyond chance. Nothing
here prints prompt text; rows are named by id only.

Run from this directory: python3 truth_read.py --reads reads.json --truths a.tsv:b.tsv
"""
import argparse
import collections
import json
import sys

MODULE = "github.com/altrace-dev-role/rashomon/"


def norm(s):
    """One spelling for a unit: no module prefix, no slashes at the ends, '-' for nothing."""
    s = (s or "").strip()
    if s.startswith(MODULE):
        s = s[len(MODULE):]
    s = s.strip("/").strip()
    return s or "-"


def parse_truth(s):
    return {norm(x) for x in s.split(";") if x.strip()} or {"-"}


def majority(reads):
    top, n = collections.Counter(reads).most_common(1)[0]
    return top if n * 2 > len(reads) else None


def judge(rows):
    out = []
    for r in rows:
        truth = parse_truth(r["truth"])
        reads = [norm(x) for x in r["reads"]]
        maj = majority(reads)
        if maj is None:
            verdict = "split"
        elif maj in truth:
            verdict = "confirmed"
        else:
            verdict = "contested"
        out.append(dict(r, reads=reads, majority=maj, verdict=verdict,
                        reader_agree=sum(1 for x in reads if x in truth),
                        unanimous=len(set(reads)) == 1))
    return out


def per_class(judged):
    table = {}
    for j in judged:
        c = table.setdefault(j["cls"], {"n": 0, "confirmed": 0, "contested": 0, "split": 0,
                                        "unanimous": 0, "reader_agree": 0, "reads": 0})
        c["n"] += 1
        c[j["verdict"]] += 1
        c["unanimous"] += int(j["unanimous"])
        c["reader_agree"] += j["reader_agree"]
        c["reads"] += len(j["reads"])
    return table


def fleiss_kappa(rows):
    """Fleiss' kappa over rows of rater labels (every row rated by the same number of raters)."""
    n, big_n = len(rows[0]), len(rows)
    per_row, totals = [], collections.Counter()
    for r in rows:
        c = collections.Counter(r)
        per_row.append((sum(v * v for v in c.values()) - n) / (n * (n - 1)))
        totals.update(c)
    p_bar = sum(per_row) / big_n
    p_e = sum((v / (big_n * n)) ** 2 for v in totals.values())
    return 1.0 if p_e == 1 else (p_bar - p_e) / (1 - p_e)


def load_truths(paths):
    truths = {}
    for path in paths:
        for line in open(path, encoding="utf-8"):
            if not line.strip() or line.startswith("#"):
                continue
            f = line.rstrip("\n").split("\t")
            if f[0] in truths and truths[f[0]] != f[1]:
                sys.exit("truth files disagree on %s: %s vs %s" % (f[0], truths[f[0]], f[1]))
            truths[f[0]] = f[1]
    return truths


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--reads", required=True)
    ap.add_argument("--truths", required=True, help="':'-separated truth tsv files")
    a = ap.parse_args(argv)
    truths = load_truths(a.truths.split(":"))
    rows = json.load(open(a.reads, encoding="utf-8"))
    missing = [r["id"] for r in rows if r["id"] not in truths]
    if missing:
        sys.exit("no truth for %s" % ",".join(missing))
    judged = judge([dict(r, truth=truths[r["id"]]) for r in rows])
    table = per_class(judged)
    width = max(len(k) for k in table)
    print("%-*s %3s %9s %9s %5s %9s %12s" % (width, "class", "n", "confirmed", "contested", "split", "unanimous", "reader_agree"))
    tot = collections.Counter()
    for cls in sorted(table):
        c = table[cls]
        tot.update(c)
        print("%-*s %3d %9d %9d %5d %9d %6d of %3d" % (width, cls, c["n"], c["confirmed"], c["contested"], c["split"], c["unanimous"], c["reader_agree"], c["reads"]))
    print("%-*s %3d %9d %9d %5d %9d %6d of %3d" % (width, "all", tot["n"], tot["confirmed"], tot["contested"], tot["split"], tot["unanimous"], tot["reader_agree"], tot["reads"]))
    print("fleiss_kappa %.3f over %d rows, %d readers" % (fleiss_kappa([j["reads"] for j in judged]), len(judged), len(judged[0]["reads"])))
    for j in judged:
        if j["verdict"] != "confirmed":
            print("%s\t%s\t%s\ttruth=%s\treads=%s" % (j["verdict"], j["id"], j["cls"], j["truth"], ";".join(j["reads"])))
    return 0


if __name__ == "__main__":
    sys.exit(main())
