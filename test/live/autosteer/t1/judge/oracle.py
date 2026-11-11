#!/usr/bin/env python3
"""A4 oracle: the blast radius of each pre-registered symbol from a synced fixture's .pgraph/pgraph.log.

    oracle.py FIXTURE_DIR [FIXTURE_DIR...]   prints {symbol id: {proven, inclusive, shape, shape_ok, reflect}}

proven    = distinct subjects of PROVEN `calls` rows whose object is the symbol
inclusive = proven plus distinct subjects of `may-call` rows whose object is the symbol
reflect   = the symbol's package has a PROVEN call to reflect.Value.MethodByName or reflect.Value.Call
Shapes (oracle-symbols.txt): proven_only p == i > 0; differ p < i; none p == i == 0; reflect p == i == 0 with a
reflect boundary in the package (the inclusive number must then never print as a bare 0).
"""
import json, os, sys

HERE = os.path.dirname(os.path.abspath(__file__))


def rows(log_path):
    out = []
    with open(log_path, encoding="utf-8") as f:
        header = f.readline().rstrip("\n")
        if header != "pgraph-log v0":
            raise SystemExit("oracle: unknown log header in " + log_path)
        for line in f:
            p = line.rstrip("\n").split("\t")
            if len(p) == 8 and p[0] == "+":
                out.append(p)
    return out


def package_of(sym):
    body = sym.split(":", 1)[1]
    return body.rsplit("/", 1)[0] + "/" + body.rsplit("/", 1)[1].split(".")[0] if "/" in body else body.split(".")[0]


def blast(live, sym):
    proven = {r[1] for r in live if r[2] == "calls" and r[3] == sym and r[4] == "PROVEN"}
    possible = {r[1] for r in live if r[2] == "may-call" and r[3] == sym}
    pkg = package_of(sym)
    reflect = any(r[2] == "calls" and r[3] in ("method:reflect.Value.MethodByName", "method:reflect.Value.Call")
                  and package_of(r[1]) == pkg for r in live)
    return len(proven), len(proven | possible), reflect


def shape_ok(shape, p, i, reflect):
    return {"proven_only": p == i > 0, "differ": p < i, "none": p == i == 0, "reflect": p == i == 0 and reflect}.get(shape, False)


def main():
    symbols = [l.rstrip("\n").split("\t") for l in open(os.path.join(HERE, "..", "oracle-symbols.txt")) if l.strip() and not l.startswith("#")]
    fixtures = {os.path.basename(d.rstrip("/")): d for d in sys.argv[1:]}
    out = {}
    for fixture, sym, _file, shape in symbols:
        d = fixtures.get(fixture)
        if d is None:
            out[sym] = {"fixture": fixture, "error": "fixture dir not given"}
            continue
        live = rows(os.path.join(d, ".pgraph", "pgraph.log"))
        p, i, reflect = blast(live, sym)
        out[sym] = {"fixture": fixture, "proven": p, "inclusive": i, "reflect": reflect, "shape": shape, "shape_ok": shape_ok(shape, p, i, reflect)}
    print(json.dumps(out, indent=1))
    return 0 if all(v.get("shape_ok") for v in out.values()) else 1


if __name__ == "__main__":
    sys.exit(main())
