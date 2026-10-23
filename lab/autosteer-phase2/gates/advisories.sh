set -eu
out="$HOME/rashomon-eval/steer/w3-security"
python3 -c '
import json,sys
pk=[l.strip().split("==") for l in open(sys.argv[1]) if "==" in l]
print(json.dumps({"queries":[{"package":{"name":n,"ecosystem":"PyPI"},"version":v} for n,v in pk]}))' "$out/freeze.txt" > "$out/q.json"
curl -sS --fail -m 60 -H 'Content-Type: application/json' -d @"$out/q.json" https://api.osv.dev/v1/querybatch > "$out/batch.json"
python3 -c '
import json,sys
q=json.load(open(sys.argv[1]))["queries"]; r=json.load(open(sys.argv[2]))["results"]
for a,b in zip(q,r):
    for v in b.get("vulns",[]): print(a["package"]["name"], a["version"], v["id"])' "$out/q.json" "$out/batch.json" > "$out/ids.txt"
: > "$out/details.ndjson"
while read -r pkg ver id; do
  curl -sS --fail -m 60 "https://api.osv.dev/v1/vulns/$id" | python3 -c '
import json,sys
d=json.load(sys.stdin); pkg,ver=sys.argv[1],sys.argv[2]
fixed=sorted({e["fixed"] for a in d.get("affected",[]) if a["package"]["name"].lower()==pkg.lower() for rg in a.get("ranges",[]) for e in rg.get("events",[]) if "fixed" in e})
print(json.dumps({"pkg":pkg,"version":ver,"id":d["id"],"cve":[x for x in d.get("aliases",[]) if x.startswith("CVE-")],"fixed":fixed,"summary":(d.get("summary") or d.get("details",""))[:100]}))' "$pkg" "$ver" >> "$out/details.ndjson"
done < "$out/ids.txt"
python3 -c '
import json,sys,subprocess
rows=[json.loads(l) for l in open(sys.argv[1])]
seen={}
print("advisories: %d across %d packages" % (len(rows), len({r["pkg"] for r in rows})))
for r in rows:
    f=(r["fixed"] or ["-"])[0]
    k=(r["pkg"],f)
    if k not in seen and f!="-":
        try:
            j=json.loads(subprocess.check_output(["curl","-sS","--fail","-m","30","https://pypi.org/pypi/%s/%s/json"%k]))
            seen[k]=j["info"].get("requires_python") or "any"
        except Exception: seen[k]="?"
    print("%-10s %-8s %-19s %-15s fixed %-8s needs-py %-7s %s" % (r["pkg"],r["version"],r["id"],",".join(r["cve"])[:15],f,seen.get(k,"-"),r["summary"]))
' "$out/details.ndjson"
