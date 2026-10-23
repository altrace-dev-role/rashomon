"""H5c: interpreter advisories from NVD's CPE dictionary match (cpeName), split by whether CPython is the
vulnerable product or only a platform another product runs on (NVD marks those vulnerable=false).
Complements H5b (OSV commit ranges). Neither sees linked OS libraries or undisclosed issues.
Usage: python3 nvd_cpython.py   (network; NVD unauthenticated limit is 5 requests per 30 s)
"""
import json
import subprocess
import time

URL = "https://services.nvd.nist.gov/rest/json/cves/2.0?cpeName=cpe:2.3:a:python:python:%s:*:*:*:*:*:*:*&resultsPerPage=2000"


def split(version):
    # curl, not urllib: the host's framework Python has no CA bundle (C49: a fetch failure must not read as 0).
    d = json.loads(subprocess.run(["curl", "-sSf", "-m", "90", URL % version], check=True,
                                  capture_output=True).stdout)
    own, platform = set(), set()
    for v in d["vulnerabilities"]:
        c = v["cve"]
        vuln = any(":python:python:" in m["criteria"] and m.get("vulnerable")
                   for cfg in c.get("configurations", []) for n in cfg.get("nodes", []) for m in n.get("cpeMatch", []))
        (own if vuln else platform).add(c["id"])
    assert len(own) + len(platform) == d["totalResults"], (version, d["totalResults"])
    return own, platform


out = {}
for ver in ("2.7.18", "3.14.9"):
    own, plat = split(ver)
    out[ver] = {"cpython_vulnerable": sorted(own), "platform_only": sorted(plat)}
    print("%-7s NVD total %d: CPython vulnerable %d, CPython only as platform %d" % (ver, len(own) + len(plat), len(own), len(plat)))
    time.sleep(7)
# Positive controls: tarfile path traversal (CVE-2007-4559) and email address parsing (CVE-2023-27043)
# are CPython's own and affect 2.7.18; a Django CVE that lists Python as its platform must split out.
assert {"CVE-2007-4559", "CVE-2023-27043"} <= set(out["2.7.18"]["cpython_vulnerable"]), out["2.7.18"]
assert "CVE-2021-32052" in out["3.14.9"]["platform_only"], out["3.14.9"]
print("controls ok")
json.dump(out, open("nvd_cpython.json", "w"), indent=1)
