# For each CVE NVD returned, say whether CPython itself is marked vulnerable (vulnerable=true on a
# python:python criteria) or appears only as a platform of another product (vulnerable=false).
import json, sys
d = json.load(open(sys.argv[1]))
own, platform = [], []
for v in d["vulnerabilities"]:
    c = v["cve"]
    vuln = False
    for cfg in c.get("configurations", []):
        for node in cfg.get("nodes", []):
            for m in node.get("cpeMatch", []):
                if ":python:python:" in m["criteria"] and m.get("vulnerable"):
                    vuln = True
    (own if vuln else platform).append(c["id"])
print(sys.argv[1], "python marked vulnerable", len(own), "| python only as platform", len(platform))
print("  own:", " ".join(sorted(own)))
print("  platform-only:", " ".join(sorted(platform)))
