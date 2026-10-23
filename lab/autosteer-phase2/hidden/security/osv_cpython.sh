# H5b: interpreter advisories by commit (OSV GIT ranges). Blind spot: only advisories OSV maps to CPython
# commits; a control on v3.8.0 returned 3 ids, far fewer than its known CVEs. Results are a lower bound.
set -u
for tag in v2.7.18 v3.14.9; do
  sha=$(git ls-remote https://github.com/python/cpython "refs/tags/$tag^{}" | cut -f1)
  [ -n "$sha" ] || sha=$(git ls-remote https://github.com/python/cpython "refs/tags/$tag" | cut -f1)
  n=$(curl -sS -X POST https://api.osv.dev/v1/query -d "{\"commit\": \"$sha\"}" | python3 -c "
import json,sys
d=json.load(sys.stdin); v=d.get('vulns',[])
ids=sorted(set(x['id'] for x in v)); al=set()
for x in v: al.update(x.get('aliases',[]))
print(len(ids), 'ids;', 'CVEs:', ' '.join(sorted(a for a in al|set(ids) if a.startswith('CVE-')))[:400])")
  echo "$tag $sha: $n"
done
# positive control: a known-vulnerable old CPython tag must return advisories
sha=$(git ls-remote https://github.com/python/cpython "refs/tags/v3.8.0^{}" | cut -f1)
echo "control v3.8.0: $(curl -sS -X POST https://api.osv.dev/v1/query -d "{\"commit\": \"$sha\"}" | python3 -c 'import json,sys; print(len(json.load(sys.stdin).get("vulns",[])), "ids")')"
