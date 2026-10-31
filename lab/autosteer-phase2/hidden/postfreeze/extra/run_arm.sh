# inside an arm image as uid 1000: /tmp/in/src is the arm's committed tree
sh /tmp/in/hostile_extra.sh /tmp/r >/dev/null 2>&1
cd /tmp/in/src
for c in $(ls /tmp/r); do
  s=$(date +%s)
  timeout 60 python -m py2scan scan --format json "/tmp/r/$c" > "/tmp/o_$c.json" 2> "/tmp/e_$c.txt"; rc=$?
  echo "$c $rc $(( $(date +%s) - s ))" >> /tmp/rc.txt
done
python - <<'PY'
import json, os
out = {}
for ln in open("/tmp/rc.txt"):
    c, rc, sec = ln.split()
    err = open("/tmp/e_%s.txt" % c, "rb").read().decode("utf-8", "replace")
    raw = open("/tmp/o_%s.json" % c, "rb").read()
    try:
        j = json.loads(raw.decode("utf-8"))
    except Exception:
        j = None
    out[c] = {"rc": int(rc), "sec": int(sec), "tb": "Traceback" in err, "err": err.strip().splitlines()[-1:] ,
              "json": j if (j is None or len(raw) < 400000) else {"findings": j.get("findings", [])[:50], "truncated": True}}
print(json.dumps(out))
PY
