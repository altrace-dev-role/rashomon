set -eu
P="$HOME/.claude/projects/-Users-karthik-rashomon-eval-py27"
O="$P-opus-py2scan/22d11efc-ea18-46c4-b0ea-93034160133c.jsonl"
S="$P-sonnet-py2scan/e0cf2733-0046-4363-bda9-d8634aaaccc8.jsonl"
B="$P-blank-py2scan/5f2242b6-7f9b-427f-b524-ac0fa9bce3c2.jsonl"
echo '--- (a) leak check: tool calls that touched the hidden oracle, the results log, or another arm'
python3 - "$O" "$S" "$B" <<'PY'
import json, sys, re
pat = re.compile(r"k-hidden-accept|ref-scan|rashomon-eval/steer|w3scan|py27/(opus|sonnet|blank)/")
for path, me in zip(sys.argv[1:], ("opus", "sonnet", "blank")):
    hits = []
    for l in open(path):
        d = json.loads(l)
        if d.get("type") != "assistant": continue
        for b in d["message"].get("content") or []:
            if b.get("type") == "tool_use":
                s = json.dumps(b.get("input"))
                for m in pat.finditer(s):
                    if m.group(0) != "py27/%s/" % me:
                        hits.append((d["timestamp"][11:19], b["name"], s[max(0, m.start()-60):m.end()+40]))
    print(me, "foreign-path tool calls:", len(hits))
    for h in hits[:4]: print("   ", h)
PY
echo '--- (b) what each arm changed against base c1e63fb (tracked diff, plus untracked files)'
for a in opus sonnet blank; do echo "== $a ($(git -C "$HOME/rashomon-eval/py27/$a/py2scan" rev-parse --abbrev-ref HEAD))"; git -C "$HOME/rashomon-eval/py27/$a/py2scan" diff --stat c1e63fb | tail -25; git -C "$HOME/rashomon-eval/py27/$a/py2scan" status --porcelain --untracked-files=all | grep '^??' || true; done
echo '--- (f) B treatment: what its CLAUDE.md says about binding or scope, and how B binds'
grep -n -i -E "bind|scope|symtable|assign|shadow" "$HOME/rashomon-eval/py27/blank/py2scan/CLAUDE.md" || echo "(no binding guidance in B's CLAUDE.md)"
grep -n -E "Param|Store|For\b|With|comprehension|arguments|vararg|kwarg|def _bound|def bound" "$HOME/rashomon-eval/py27/blank/py2scan/py2scan/rules.py" | head -12
