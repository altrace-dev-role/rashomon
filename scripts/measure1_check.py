#!/usr/bin/env python3
"""measure1_check.py -- the JSON reads scripts/measure1.sh cannot do safely
in sh (card MEASURE1). Each subcommand reads files, prints what it found and
exits 0 when its premise holds, 1 when it does not (the driver stops the run),
2 on bad arguments. It writes only the files it is told to write (prereg,
trust). Python 3 standard library only.
"""

import glob
import hashlib
import json
import os
import random
import sys
from typing import NoReturn

# S2's markers: every model-facing string rashomon renders starts with
# render.MessagePrefix, and recap's line carries " rashomon: ".
# test/acceptance's TestMS1y pins the first to the Go constant.
MARKERS = ("rashomon record: ", " rashomon: ")

# G1b compares these init keys across the arms; the rest name the arm's own
# paths or ids.
INIT_DIFF = ("tools", "mcp_servers", "plugins", "model", "slash_commands", "output_style",
             "apiKeySource", "skills", "agents", "permissionMode", "claude_code_version",
             "capabilities", "fast_mode_state", "analytics_disabled")


def fail(msg) -> NoReturn:
    print("measure1: " + msg)
    sys.exit(1)


def lines(path, offset=0):
    """The complete JSON lines of path past byte offset, decoded; a torn last
    line is not yet a line."""
    try:
        with open(path, "rb") as f:
            f.seek(offset)
            data = f.read()
    except FileNotFoundError:
        return []
    out = []
    for raw in data.split(b"\n")[:-1]:
        try:
            out.append(json.loads(raw))
        except ValueError:
            continue
    return out


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 16), b""):
            h.update(chunk)
    return h.hexdigest()


def cmd_prereg(args):
    """prereg OUT KEY=VALUE... -- FILE...: the pre-registration, frozen
    before G0 ends."""
    out, rest = args[0], args[1:]
    sep = rest.index("--") if "--" in rest else len(rest)
    doc = dict(kv.split("=", 1) for kv in rest[:sep])
    doc["sha256"] = {p: sha256(p) for p in sorted(rest[sep + 1:])}
    with open(out, "w") as f:
        json.dump(doc, f, indent=1, sort_keys=True)
        f.write("\n")


def cmd_order(args):
    """order SEED REPS: the arms in their seeded order, one per line."""
    rng = random.Random(int(args[0]))
    for _ in range(int(args[1])):
        pair = ["off", "steer"]
        rng.shuffle(pair)
        print("\n".join(pair))


def cmd_envdiff(args):
    """envdiff DUMP_A DIR_A WS_A SID_A DUMP_B DIR_B WS_B SID_B: the hook-side
    dumps of G1a, each arm's workspace, directory and session id normalised;
    only RASHOMON_STEER_ARM may differ."""
    def read(path, d, ws, sid):
        with open(path) as f:
            text = f.read()
        for real, name in ((os.path.realpath(ws), "<WS>"), (ws, "<WS>"), (os.path.realpath(d), "<ARM>"), (d, "<ARM>"), (sid, "<SESSION>")):
            text = text.replace(real, name)
        return text.splitlines()
    a, b = read(*args[0:4]), read(*args[4:8])
    da = sorted(set(a) - set(b))
    db = sorted(set(b) - set(a))
    if any(not l.startswith("RASHOMON_STEER_ARM=") for l in da + db) or len(da) != 1 or len(db) != 1:
        fail("G1a: the arms' environments differ beyond RASHOMON_STEER_ARM: %s against %s" % (da, db))
    print("measure1: G1a %d lines each, equal but for %s and %s" % (len(a), da[0], db[0]))


def init_event(path) -> dict:
    events = lines(path)
    for ev in events:
        if ev.get("type") == "assistant" and ev.get("error") == "authentication_failed":
            fail("G1b: %s is not logged in: the template's login does not reach the arm" % path)
    for ev in events:
        if ev.get("type") == "system" and ev.get("subtype") == "init":
            return ev
    fail("G1b: no init event in %s" % path)


def cmd_init(args):
    """init STREAM_A ARM_A STREAM_B ARM_B MODEL VERSION: G1b's premises on
    each arm's init event, then the two diffed."""
    model, version = args[4], args[5]
    evs = []
    for stream, arm in ((args[0], args[1]), (args[2], args[3])):
        ev = init_event(stream)
        if ev.get("claude_code_version") != version:
            fail("G1b: %s reports claude %r, not %s" % (stream, ev.get("claude_code_version"), version))
        if not ev.get("tools"):
            fail("G1b: %s lists no tools" % stream)
        if ev.get("model") != model:
            fail("G1b: %s runs model %r, not %s" % (stream, ev.get("model"), model))
        if ev.get("mcp_servers"):
            fail("G1b: %s has MCP servers %r under --strict-mcp-config" % (stream, ev.get("mcp_servers")))
        sock = ev.get("messaging_socket_path") or ""
        if sock and not os.path.realpath(sock).startswith(os.path.realpath(arm) + os.sep):
            fail("G1b: %s's messaging socket %s is outside its arm %s" % (stream, sock, arm))
        evs.append(ev)
    diff = [k for k in INIT_DIFF if evs[0].get(k) != evs[1].get(k)]
    if diff:
        fail("G1b: the arms' init events differ in %s" % ", ".join(diff))
    tools = evs[0]["tools"]
    print("measure1: G1b init events equal in %d keys; claude %s, model %s, %d tools, apiKeySource %s"
          % (len(INIT_DIFF), version, model, tools if isinstance(tools, int) else len(tools), evs[0].get("apiKeySource")))


def cmd_claudemd(args):
    """claudemd WS CONFIG MANAGED: every CLAUDE.md Claude Code would read
    for WS other than WS's own."""
    ws, cfg, managed = (os.path.realpath(a) for a in args[:3])
    found = [p for p in (os.path.join(cfg, "CLAUDE.md"), os.path.join(managed, "CLAUDE.md")) if os.path.exists(p)]
    d = os.path.dirname(ws)
    while True:
        for name in ("CLAUDE.md", "CLAUDE.local.md"):
            p = os.path.join(d, name)
            if os.path.exists(p):
                found.append(p)
        if d == os.path.dirname(d):
            break
        d = os.path.dirname(d)
    if found:
        fail("G1d: memory files outside the workspace: %s" % ", ".join(found))
    print("measure1: G1d no CLAUDE.md outside %s (config %s, managed %s, %s's ancestors)" % (ws, cfg, managed, ws))


def cmd_repoclaude(args):
    """repoclaude WS: the task repository's own Claude Code files, dumped; a
    hook that is not rashomon's stops the task (G1e)."""
    ws = args[0]
    for rel in (".claude/settings.json", ".claude/settings.local.json", "CLAUDE.md", ".mcp.json"):
        p = os.path.join(ws, rel)
        print("measure1: G1e %s %s" % (rel, sha256(p) if os.path.exists(p) else "absent"))
    for rel in (".claude/settings.json", ".claude/settings.local.json"):
        p = os.path.join(ws, rel)
        if not os.path.exists(p):
            continue
        with open(p) as f:
            hooks = json.load(f).get("hooks") or {}
        for event, groups in hooks.items():
            for g in groups:
                for h in g.get("hooks", []):
                    if "rashomon" not in h.get("command", ""):
                        fail("G1e: %s holds a %s hook that is not rashomon's: %r" % (rel, event, h.get("command")))


def cmd_trust(args):
    """trust CCDIR WS: hasTrustDialogAccepted for WS in CCDIR/.claude.json,
    declared and the same in both arms."""
    path = os.path.join(args[0], ".claude.json")
    doc = {}
    if os.path.exists(path):
        with open(path) as f:
            doc = json.load(f)
    doc.setdefault("projects", {}).setdefault(os.path.realpath(args[1]), {})["hasTrustDialogAccepted"] = True
    with open(path, "w") as f:
        json.dump(doc, f, indent=2)


def cmd_addhook(args):
    """addhook SETTINGS COMMAND: one more SessionStart command in a probe's
    settings copy (G1a's hook-side dump). Never used on a run arm."""
    with open(args[0]) as f:
        doc = json.load(f)
    doc.setdefault("hooks", {}).setdefault("SessionStart", []).append(
        {"hooks": [{"type": "command", "command": args[1], "timeout": 30}]})
    with open(args[0], "w") as f:
        json.dump(doc, f, indent=2)


def cmd_uuid(args):
    """uuid: a fresh session id."""
    import uuid
    print(uuid.uuid4())


def transcript_of(ccdir, session):
    found = glob.glob(os.path.join(ccdir, "projects", "*", session + ".jsonl"))
    return found[0] if found else ""


def cmd_transcript(args):
    """transcript CCDIR SESSION: the session's main transcript path, or
    nothing."""
    print(transcript_of(args[0], args[1]))


def user_text(entry):
    c = (entry.get("message") or {}).get("content")
    if isinstance(c, list):
        c = "".join(p.get("text", "") for p in c if isinstance(p, dict) and p.get("type") == "text")
    return c if isinstance(c, str) else None


def cmd_submitted(args):
    """submitted TRANSCRIPT OFFSET PROMPT_FILE: exit 0 when a user entry past
    OFFSET carries the prompt's text, 1 when none has arrived yet, 3 when one
    arrived with other text (a resend could submit it twice)."""
    with open(args[2]) as f:
        want = hashlib.sha256(f.read().encode()).hexdigest()
    texts = [user_text(e) for e in lines(args[0], int(args[1])) if e.get("type") == "user"]
    texts = [t for t in texts if t is not None]
    if not texts:
        sys.exit(1)
    if any(hashlib.sha256(t.encode()).hexdigest() == want for t in texts):
        sys.exit(0)
    print("measure1: a user entry arrived whose text is not the prompt's")
    sys.exit(3)


def cmd_turnend(args):
    """turnend RECORDS OFFSET TRANSCRIPT TOFFSET: exit 0 and print the end
    type when a complete stop checkpoint was appended past OFFSET;
    1 when none has been."""
    stops = [r for r in lines(args[0], int(args[1])) if r.get("type") == "verify" and r.get("point") == "stop"]
    if not stops:
        sys.exit(1)
    api = [e for e in lines(args[2], int(args[3])) if e.get("type") == "assistant" and e.get("isApiErrorMessage")]
    print("stop_failure" if api else "stop")


def cmd_pending(args):
    """pending RECORDS: what a silent turn waits on. A declaration with no
    execution is a call that has not returned; PreToolUse runs before the
    permission flow, so that is a permission prompt or a call still running,
    and the pane capture tells which. The hook's own terminal closes the hook
    process, not the call, so it is no return. With no call pending, the
    model itself is silent."""
    recs = lines(args[0])
    decl = [r.get("tool_use_id") for r in recs if r.get("type") == "declaration"]
    done = {r.get("tool_use_id") for r in recs if r.get("type") == "execution"}
    print("call pending: permission prompt or long call" if decl and decl[-1] not in done else "hang: no call pending")


def steer_lines(store, session):
    return lines(os.path.join(store, "runs", session, "steer.ndjson"))


def cmd_control(args):
    """control STORE_STEER SESSION_STEER STORE_OFF SESSION_OFF: G1c's
    positive control. Prints the delivered note's model text for the
    channel check."""
    on, off = steer_lines(args[0], args[1]), steer_lines(args[2], args[3])
    # The channel first: a probe that logged nothing and one whose lines
    # never reach the model are the same finding, one treatment in both arms.
    got = [l for l in on if l.get("delivery") == "delivered" and l.get("level") == "nudge" and l.get("model_text")]
    if not got:
        fail("G1c: no model channel on this tree: the steer arm delivered no nudge with model text (%d steer lines logged)" % len(on))
    for name, ls in (("steer", on), ("off", off)):
        if not ls:
            fail("G1c: the %s arm's probe logged no steer line" % name)
        for l in ls:
            if l.get("surface") != "cli" or not l.get("prompt_id"):
                fail("G1c: a %s line has surface %r and prompt_id %r, not cli and an id" % (name, l.get("surface"), l.get("prompt_id")))
    c = got[0]
    twin = [l for l in off if l.get("pattern") == c["pattern"] and l.get("key") == c["key"]
            and l.get("suppressed_by") == "arm_off" and l.get("level") == "nudge" and l.get("arm") == "off"]
    if not twin:
        fail("G1c: the off arm has no %s/%s line withheld by arm_off at nudge" % (c["pattern"], c["key"]))
    print(c["model_text"])


def cmd_holds(args):
    """holds TRANSCRIPT TEXT: exit 0 when the transcript's bytes hold TEXT
    (G1c's channel check)."""
    with open(args[0], "rb") as f:
        data = f.read().decode("utf-8", "replace")
    sys.exit(0 if args[1] and (args[1] in data or json.dumps(args[1])[1:-1] in data) else 1)


def cmd_sensors(args):
    """sensors ARM STORE SESSION TRANSCRIPT CONTROL FORBIDDEN...: S1 over the
    steer log, S2 over the transcript (the off arm's must hold no marker; the
    steer arm's must hold one when CONTROL is 1, G1c having found the channel
    in the transcript) and S4 over the transcript's tool inputs. Prints what
    each scanned."""
    arm, store, session, tr, control, forbidden = args[0], args[1], args[2], args[3], args[4] == "1", args[5:]
    ls = steer_lines(store, session)
    bad = [l for l in ls if l.get("arm") != arm or l.get("delivery") == "holdout"
           or (arm == "off" and l.get("delivery") in ("delivered", "flag_only"))]
    print("measure1: S1 %s arm: %d steer lines scanned, %d out of arm" % (arm, len(ls), len(bad)))
    entries, text = [], ""
    if tr:
        entries = lines(tr)
        with open(tr, "rb") as f:
            text = f.read().decode("utf-8", "replace")
    marks = sum(text.count(m) for m in MARKERS)
    print("measure1: S2 %s arm: %d transcript lines scanned, %d rashomon markers" % (arm, len(entries), marks))
    reach = 0
    for e in entries:
        for part in (e.get("message") or {}).get("content") or []:
            if isinstance(part, dict) and part.get("type") == "tool_use":
                blob = json.dumps(part.get("input"))
                reach += sum(1 for p in forbidden if p and p in blob)
    print("measure1: S4 %s arm: %d tool inputs naming a path outside the arm" % (arm, reach))
    if bad or reach or (arm == "off" and marks):
        fail("G5: the %s arm fails a sensor" % arm)
    if arm == "steer" and control and not marks:
        fail("G5: S2's control failed: the steer transcript holds no rashomon marker, though G1c found the channel there")


def usage_of(tr):
    """requests, context tokens, cache reads and output in a transcript.
    Claude Code writes one entry per content block, each with its
    message's usage, so a message id counts once; an entry with none
    counts as its own."""
    seen, r, ctx, cr, out = set(), 0, 0, 0, 0
    for e in lines(tr):
        m = e.get("message") or {}
        u = m.get("usage")
        if e.get("type") != "assistant" or not u:
            continue
        mid = m.get("id")
        if mid and mid in seen:
            continue
        seen.add(mid)
        r += 1
        c = u.get("cache_read_input_tokens", 0)
        ctx += u.get("input_tokens", 0) + u.get("cache_creation_input_tokens", 0) + c
        cr += c
        out += u.get("output_tokens", 0)
    return r, ctx, cr, out


def cmd_projection(args):
    """projection PROMPTS TRANSCRIPT...: G1c's probes ran prompt 1, so their
    usage is the per-prompt figure before any arm spends; prints it per
    probe and the arms' projection, prompt 1 times the prompts times two."""
    n, trs = int(args[0]), [t for t in args[1:] if t]
    if not trs:
        print("measure1: G1c no probe transcript, so no projection")
        return
    tot = [sum(x) for x in zip(*(usage_of(t) for t in trs))]
    r, ctx, cr, out = (v / len(trs) for v in tot)
    print("measure1: G1c prompt 1 per probe: %.0f request(s), %.0f context tokens (%.0f%% cache reads), %.0f output; "
          "projected arms (x%d prompts x2 arms): %.0f requests, %.0f context tokens, %.0f output"
          % (r, ctx, 100 * cr / ctx if ctx else 0, out, n, r * n * 2, ctx * n * 2, out * n * 2))


def cmd_markers(args):
    """markers: S2's markers, one JSON string a line, for the driver test
    that pins them to the binary's own prefixes."""
    for m in MARKERS:
        print(json.dumps(m))


def cmd_oracle(args):
    """oracle JSON F2P P2P MODE: score go test -json output. F2P lines are
    "<package> <Test>", P2P lines a package. MODE red: every F2P test emits
    its own fail (a build failure is not one: go reports it as the package's
    fail with FailedBuild set, the build-fail event naming only ImportPath).
    MODE green: every F2P test passes, and no P2P test fails apart from
    them, nor any P2P package's build. MODE score: print the counts only."""
    events = lines(args[0])
    f2p = [l.split() for l in open(args[1]).read().splitlines() if l.strip()]
    p2p = [l.strip() for l in open(args[2]).read().splitlines() if l.strip()]
    tests, pkgs = {}, {}
    for e in events:
        act = e.get("Action")
        if e.get("Test") and act in ("pass", "fail", "skip"):
            tests[(e.get("Package", ""), e["Test"])] = act
        elif not e.get("Test") and e.get("Package") and act in ("pass", "fail", "skip"):
            pkgs[e["Package"]] = "build-fail" if act == "fail" and e.get("FailedBuild") else act

    def in_pkg(p, q):
        return p == q or p.endswith("/" + q.lstrip("./")) or q in (".", "./")

    def match(pkg, name):
        return [v for (p, n), v in tests.items() if n == name and in_pkg(p, pkg)]

    def is_f2p(p, n):
        return any(in_pkg(p, fp) and (n == fn or n.startswith(fn + "/")) for fp, fn in f2p)
    f2p_pass = sum(1 for p, n in f2p if match(p, n) == ["pass"])
    f2p_fail = sum(1 for p, n in f2p if match(p, n) == ["fail"])
    # PASS_TO_PASS is scored per test, the FAIL_TO_PASS tests and their
    # subtests apart: in a one-package task the package holds both, and a
    # package verdict would count a failed fix as a regression too.
    p2p_bad = sorted("%s %s" % (p, n) for (p, n), v in tests.items()
                     if v == "fail" and any(in_pkg(p, q) for q in p2p) and not is_f2p(p, n))
    unbuilt = sorted(p for p, v in pkgs.items() if v == "build-fail")
    p2p_bad += [p + " (did not build)" for p in unbuilt if any(in_pkg(p, q) for q in p2p)]
    print("measure1: oracle FAIL_TO_PASS %d/%d passed, %d failed on their assertions; PASS_TO_PASS %d failing apart from them; %d package(s) did not build"
          % (f2p_pass, len(f2p), f2p_fail, len(p2p_bad), len(unbuilt)))
    if args[3] == "red" and f2p_fail != len(f2p):
        why = "; %s did not build, so its tests never ran" % ", ".join(unbuilt) if unbuilt else ""
        fail("G2: on base %d of %d FAIL_TO_PASS tests fail on their own assertions%s; the task is rejected" % (f2p_fail, len(f2p), why))
    if args[3] == "green" and (f2p_pass != len(f2p) or p2p_bad):
        fail("G2: the reference does not pass: FAIL_TO_PASS %d/%d, PASS_TO_PASS failing %s" % (f2p_pass, len(f2p), p2p_bad))


COMMANDS = {
    "prereg": cmd_prereg, "order": cmd_order, "envdiff": cmd_envdiff, "init": cmd_init,
    "claudemd": cmd_claudemd, "repoclaude": cmd_repoclaude, "trust": cmd_trust,
    "addhook": cmd_addhook, "uuid": cmd_uuid,
    "transcript": cmd_transcript, "submitted": cmd_submitted, "turnend": cmd_turnend,
    "pending": cmd_pending, "control": cmd_control, "holds": cmd_holds, "sensors": cmd_sensors,
    "oracle": cmd_oracle, "markers": cmd_markers, "projection": cmd_projection,
}

if __name__ == "__main__":
    if len(sys.argv) < 2 or sys.argv[1] not in COMMANDS:
        print("usage: measure1_check.py %s ..." % "|".join(sorted(COMMANDS)), file=sys.stderr)
        sys.exit(2)
    COMMANDS[sys.argv[1]](sys.argv[2:])
