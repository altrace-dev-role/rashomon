cd /tmp/in/src
for c in hexc zlibc b64 rot utf16 puny; do
  s=$(date +%s)
  timeout 60 python -m py2scan scan --format json /tmp/in/tree/$c > /tmp/o_$c.json 2> /tmp/e_$c.txt; rc=$?
  e=$(( $(date +%s) - s ))
  python - "$c" "$rc" "$e" <<'PY'
import json, sys
c, rc, e = sys.argv[1:4]
err = open("/tmp/e_%s.txt" % c).read()
tb = "Traceback" in err
try:
    r = json.load(open("/tmp/o_%s.json" % c))
except Exception:
    print("%-6s rc=%s %ss tb=%s json=INVALID err=%s" % (c, rc, e, tb, err.strip().splitlines()[-1:] )); sys.exit()
J = ["3.6","3.7","3.8","3.9","3.10","3.11","3.12","3.13","3.14","3.15"]
vers = set()
for f in r.get("findings", []):
    if f.get("file","").endswith("t.py") and f.get("severity") == "blocker":
        vers.update(f.get("versions") or J)
miss = [v for v in J if v not in vers]
named = "t.py" in json.dumps(r)
print("%-6s rc=%s %ss tb=%s t.py-blocker-missing-for=%s named=%s rules=%s" % (c, rc, e, tb, (miss[0]+".."+miss[-1]+"(%d)" % len(miss)) if miss else "none", named,
      sorted({f.get("rule") for f in r.get("findings",[]) if f.get("file","").endswith("t.py")})))
PY
done
