"""Manifest digest of a Go cache base (G-CACHE amendment pin).

One line per regular file, sorted by relative path: "<relpath> <size> <sha256>".
Prints the file count, total bytes and the sha256 of the manifest text, and
writes the manifest to argv[2] (refuses if it exists).
argv: base dir, manifest output path.
"""
import hashlib
import os
import sys

base, out = sys.argv[1], sys.argv[2]
if os.path.exists(out):
    sys.exit(f"REFUSED: {out} exists")
lines, total = [], 0
for dp, dns, fns in os.walk(base):
    dns.sort()
    for fn in fns:
        p = os.path.join(dp, fn)
        if not os.path.isfile(p) or os.path.islink(p):
            continue
        h = hashlib.sha256()
        with open(p, "rb") as f:
            for chunk in iter(lambda: f.read(1 << 20), b""):
                h.update(chunk)
        size = os.path.getsize(p)
        total += size
        lines.append(f"{os.path.relpath(p, base)} {size} {h.hexdigest()}")
lines.sort()
text = "\n".join(lines) + "\n"
with open(out, "x") as f:
    f.write(text)
print(f"files {len(lines)} bytes {total} manifest_sha256 {hashlib.sha256(text.encode()).hexdigest()}")
