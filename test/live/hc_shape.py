#!/usr/bin/env python3
"""The one reader of what the live hook-contract checks capture.

test/live/hc-items.sh and test/live/p-tru-1.sh run real Claude Code sessions
and hand this file every payload, stream and log they produce. Nothing else in
those checks looks inside one, for the reason internal/shape is the only code
in the program that looks inside tool_input: one file to audit for the claim
that nothing kept or printed is a value. What leaves here is a key name Claude Code's schema gives
it, a count, a size, an exit code, a yes or no, a value from a closed list, or
a digest that is compared and never printed. A prompt, a command, a tool's
output, an error's text, a path, the account's tools and plugins: none of it.
H-115 hands this file inputs whose every value is a canary and fails on any
canary in what it writes.

    record LOG TAG          a hook: reads the payload on stdin, appends its shape to LOG
    hooks STREAM            the hook_response events of a stream-json run
    hc1 .. hc6, fm11 .. fm13   one check's summary; see each function
    ptru1 STREAM TRANSCRIPT EVENT CTX SM FIRED   p-tru-1.sh's verdict on one event

Run it with python3 -I, as the script does: it reads nothing from the
environment but the two variables named below, and imports nothing local.
"""
import collections
import glob
import hashlib
import json
import os
import re
import sys
import time

EVENTS = {"SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse", "PostToolUse",
          "PostToolUseFailure", "Stop", "SubagentStop", "PreCompact", "PostCompact", "Notification"}
SOURCES = {"startup", "resume", "clear", "compact", "fork"}
TRIGGERS = {"manual", "auto"}
OUTCOMES = {"success", "error", "cancelled", "blocked", "non_blocking_error"}
SUBTYPES = {"success", "error_max_turns", "error_during_execution", "error"}
DESTINATIONS = {"userSettings", "projectSettings", "localSettings", "flagSettings", "policySettings"}
ATTACHMENTS = ("hook_additional_context", "hook_system_message", "hook_success")
# A top-level key is printed only when it looks like one of Claude Code's own.
KEY = re.compile(r"^[a-z][a-z0-9_]{0,39}$")
RUN = re.compile(r"^[a-z0-9]{1,8}$")


def closed(v, allowed):
    """v when it is one of allowed, "other" when it is anything else, None when absent."""
    if v is None:
        return None
    return v if isinstance(v, str) and v in allowed else "other"


def digest(v):
    """A value compared for equality and never printed: prompt_id, tool_use_id, session_id."""
    if not isinstance(v, str) or not v:
        return None
    return hashlib.sha256(v.encode("utf-8")).hexdigest()[:16]


def yn(b):
    return "yes" if b else "no"


def num(v):
    return str(v) if isinstance(v, int) and not isinstance(v, bool) else "unknown"


def size(v):
    return str(len(v.encode("utf-8"))) if isinstance(v, str) else "unknown"


def case_of(command):
    """Which of the commands a check asked for this call ran. The command is read here and dropped."""
    if not isinstance(command, str):
        return None
    c = command.strip()
    if "git commit" in c:
        return "commit"
    if c == "false":
        return "false"
    if c == "exit 3":
        return "exit3"
    if c.startswith("echo "):
        return "echo"
    return "other"


def error_shape(err, marker):
    """PostToolUseFailure's error, as its first line's exit code and how much follows it."""
    if not isinstance(err, str):
        return None
    lines = err.split("\n")
    m = re.fullmatch(r"Exit code (\d+)", lines[0])
    return {"exit_code": int(m.group(1)) if m else None,
            "further_lines": len(lines) - 1,
            "carries_marker": bool(marker) and marker in err}


def record(log, tag):
    """One hook invocation. It prints nothing and exits 0 whatever happens: it is a hook."""
    run = os.environ.get("HC_RUN", "")
    rec = {"tag": tag if RUN.match(tag or "") else "other",
           "run": run if RUN.match(run) else "other",
           "t": time.time()}
    try:
        p = json.loads(sys.stdin.read())
        if not isinstance(p, dict):
            raise ValueError("payload is not an object")
        ti = p.get("tool_input")
        rec.update({
            "event": closed(p.get("hook_event_name"), EVENTS),
            "keys": sorted(k if KEY.match(k) else "<other>" for k in p),
            "session": digest(p.get("session_id")),
            "prompt": digest(p.get("prompt_id")),
            "tool_use": digest(p.get("tool_use_id")),
            "source": closed(p.get("source"), SOURCES),
            "trigger": closed(p.get("trigger"), TRIGGERS),
            "case": case_of(ti.get("command")) if isinstance(ti, dict) else None,
            "error": error_shape(p.get("error"), os.environ.get("HC_MARKER", "")),
            "is_interrupt": p["is_interrupt"] if isinstance(p.get("is_interrupt"), bool) else None,
        })
    except Exception as e:  # noqa: BLE001 -- a hook that raises is a hook that failed out loud
        rec.update({"event": "record-failed", "error_class": type(e).__name__})
    try:
        with open(log, "a", encoding="utf-8") as f:
            f.write(json.dumps(rec, sort_keys=True) + "\n")
    except OSError:
        pass
    return 0


def jsonl(path):
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            lines = f.read().splitlines()
    except OSError:
        return None
    out = []
    for ln in lines:
        try:
            v = json.loads(ln)
        except ValueError:
            continue
        if isinstance(v, dict):
            out.append(v)
    return out


def whole_json(path):
    """--output-format json's one object, however it is laid out."""
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            v = json.loads(f.read())
        return v if isinstance(v, dict) else None
    except (OSError, ValueError):
        return (jsonl(path) or [None])[0]


def read_int(path):
    try:
        with open(path, encoding="utf-8") as f:
            return int(f.read().strip())
    except (OSError, ValueError):
        return None


def hook_name(v):
    """hook_response's hook_name is "<Event>" or "<Event>:<source>"; anything else prints as other."""
    if not isinstance(v, str):
        return "other"
    event, _, source = v.partition(":")
    if event not in EVENTS or (source and source not in SOURCES):
        return "other"
    return v


def hooks(stream):
    events = jsonl(stream) or []
    n = 0
    for e in events:
        if e.get("type") == "system" and e.get("subtype") == "hook_response":
            n += 1
            print("  hook_response %s: exit_code %s, outcome %s, stdout %s bytes, stderr %s bytes" % (
                hook_name(e.get("hook_name")), num(e.get("exit_code")), closed(e.get("outcome"), OUTCOMES),
                size(e.get("stdout")), size(e.get("stderr"))))
    if n == 0:
        print("  hook_response events: none")
    return 0


def result_of(events):
    for e in events or []:
        if e.get("type") == "result":
            return e
    return None


def text_of(e):
    return e.get("result") if isinstance(e, dict) and isinstance(e.get("result"), str) else None


def has(text, token):
    return "unknown" if not token or text is None else yn(token in text)


def hc1(stream, transcript, ctx_start, sm_start, ctx_ups, sm_ups):
    """Which half of one {systemMessage, hookSpecificOutput.additionalContext} object reached the model."""
    events = jsonl(stream) or []
    hooks(stream)
    reply = text_of(result_of(events))
    if reply is None:
        print("  the model's reply: not observed")
    else:
        print("  the model's reply names: SessionStart additionalContext %s, SessionStart systemMessage %s, "
              "UserPromptSubmit additionalContext %s, UserPromptSubmit systemMessage %s" % (
                  has(reply, ctx_start), has(reply, sm_start), has(reply, ctx_ups), has(reply, sm_ups)))
    info = [e.get("content") for e in events if e.get("type") == "system" and e.get("subtype") == "informational"]
    joined = " ".join(c for c in info if isinstance(c, str))
    print("  stream system/informational events: %d; carrying SessionStart's systemMessage %s, UserPromptSubmit's %s" % (
        len(info), has(joined, sm_start), has(joined, sm_ups)))
    recs = jsonl(transcript)
    if recs is None:
        print("  session transcript: not found")
        return 0
    counts = collections.Counter()
    for r in recs:
        a = r.get("attachment")
        if isinstance(a, dict) and a.get("type") in ATTACHMENTS and a.get("hookEvent") in EVENTS:
            counts[(a["type"], a["hookEvent"])] += 1
    print("  transcript attachments: " + "; ".join(
        "%s SessionStart %d UserPromptSubmit %d" % (t, counts[(t, "SessionStart")], counts[(t, "UserPromptSubmit")])
        for t in ATTACHMENTS))
    return 0


def by_run(log, tag=None):
    runs = collections.OrderedDict()
    for r in jsonl(log) or []:
        if tag is None or r.get("tag") == tag:
            runs.setdefault(r.get("run"), []).append(r)
    return runs


def turns(recs):
    """Each record paired with the prompt_id digest of the UserPromptSubmit before it in the same run."""
    current = None
    for r in recs:
        if r.get("event") == "UserPromptSubmit":
            current = r.get("prompt")
        yield r, current


def hc2(log):
    """prompt_id on UserPromptSubmit against the same turn's PreToolUse, PostToolUse and Stop."""
    runs = by_run(log)
    if not runs:
        print("  hook records: none")
        return 0
    ups = {}
    for run, recs in runs.items():
        n = collections.Counter(r.get("event") for r in recs)
        tools = {r.get("tool_use") for r in recs if r.get("event") == "PreToolUse" and r.get("tool_use")}
        ups[run] = [r.get("prompt") for r in recs if r.get("event") == "UserPromptSubmit"]
        eq, tot = collections.Counter(), collections.Counter()
        for r, turn in turns(recs):
            if r.get("event") in ("PreToolUse", "PostToolUse", "Stop"):
                tot[r["event"]] += 1
                eq[r["event"]] += 1 if turn and r.get("prompt") == turn else 0
        print("  run %s: UserPromptSubmit %d, PreToolUse %d, PostToolUse %d, Stop %d; distinct tool_use_id over PreToolUse %d" % (
            run, n["UserPromptSubmit"], n["PreToolUse"], n["PostToolUse"], n["Stop"], len(tools)))
        print("    prompt_id equal to that turn's UserPromptSubmit: PreToolUse %d/%d, PostToolUse %d/%d, Stop %d/%d" % (
            eq["PreToolUse"], tot["PreToolUse"], eq["PostToolUse"], tot["PostToolUse"], eq["Stop"], tot["Stop"]))
    a, b = ups.get("1") or [None], ups.get("2") or [None]
    differ = "unknown" if a[0] is None or b[0] is None else yn(a[0] != b[0])
    print("  UserPromptSubmit prompt_id differs between run 1 and run 2: " + differ)
    seen = {}
    for recs in runs.values():
        for r in recs:
            seen.setdefault(r.get("event"), r.get("keys") or [])
    for ev in ("SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"):
        if ev in seen:
            print("  keys %s: %s" % (ev, ", ".join(seen[ev])))
    return 0


def fm12(log):
    """The Stop payload carries prompt_id, and it is its turn's."""
    stops = carrying = equal = 0
    for recs in by_run(log).values():
        for r, turn in turns(recs):
            if r.get("event") == "Stop":
                stops += 1
                carrying += 1 if r.get("prompt") else 0
                equal += 1 if turn and r.get("prompt") == turn else 0
    print("  Stop payloads %d, carrying prompt_id %d, equal to their turn's UserPromptSubmit prompt_id %d" % (stops, carrying, equal))
    return 0


def fm13(log):
    """SessionStart's source on each run, and the order of events around a manual /compact (run 3)."""
    runs = by_run(log)
    starts = []
    for run, recs in runs.items():
        sources = [r.get("source") or "none" for r in recs if r.get("event") == "SessionStart"]
        if sources:
            starts.append("run %s %s" % (run, "+".join(sources)))
    print("  SessionStart source by run: " + (", ".join(starts) or "none"))
    third = runs.get("3", [])

    def label(r):
        extra = ["%s %s" % (k, r[k]) for k in ("source", "trigger") if r.get(k)]
        return "%s(%s)" % (r.get("event"), ", ".join(extra)) if extra else str(r.get("event"))
    print("  run 3 events in order: " + (", ".join(label(r) for r in third[:12]) or "none"))
    compact = [r for r in third if r.get("event") == "SessionStart" and r.get("source") == "compact"]
    pre = [r for r in third if r.get("event") == "PreCompact"]
    if not compact:
        print("  compact-source SessionStart: not observed")
    else:
        pid = compact[0].get("prompt")
        same = "unknown" if not pre else yn(bool(pid) and pid == pre[0].get("prompt"))
        print("  compact-source SessionStart carries prompt_id %s; equal to PreCompact's %s" % (yn(pid), same))
    return 0


CASES = (("commit", "hook-rejected git commit"), ("false", "silent failure (false)"), ("exit3", "exit 3"))


def hc3(log):
    """The PostToolUseFailure error string's shape for each of the three failing commands."""
    recs = [r for rs in by_run(log).values() for r in rs]
    for case, name in CASES:
        mine = [r for r in recs if r.get("case") == case]
        n = collections.Counter(r.get("event") for r in mine)
        print("  %s: PreToolUse %d, PostToolUseFailure %d, PostToolUse %d" % (
            name, n["PreToolUse"], n["PostToolUseFailure"], n["PostToolUse"]))
        for r in mine:
            e = r.get("error")
            if r.get("event") != "PostToolUseFailure" or not isinstance(e, dict):
                continue
            print("    error: first line is 'Exit code N' %s, N=%s, further lines %s, carries the commit-msg hook's line %s; "
                  "tool_response key %s; is_interrupt %s" % (
                      yn(e.get("exit_code") is not None), num(e.get("exit_code")), num(e.get("further_lines")),
                      yn(e.get("carries_marker")), yn("tool_response" in (r.get("keys") or [])),
                      "unknown" if r.get("is_interrupt") is None else yn(r["is_interrupt"])))
    keys = next((r.get("keys") for r in recs if r.get("event") == "PostToolUseFailure"), None)
    print("  keys PostToolUseFailure: " + (", ".join(keys) if keys else "not observed"))
    return 0


def init_of(events):
    for e in events or []:
        if e.get("type") == "system" and e.get("subtype") == "init":
            return e
    return None


def setting_role(path, cfg, cwd):
    if path == os.path.join(cfg, "settings.json"):
        return "user"
    if path.endswith("/managed-settings.json"):
        return "managed"
    if path == os.path.join(cwd, ".claude", "settings.json"):
        return "project"
    if path == os.path.join(cwd, ".claude", "settings.local.json"):
        return "local"
    return None


def hc5(d, cfg, cwd, n1):
    """A nested headless run from inside a Stop hook: did it run, tool-less, under its own id, with whose hooks and settings."""
    rc = read_int(os.path.join(d, "n1.exit"))
    if rc is None:
        print("  nested run from the outer Stop hook: did not run")
        return 0
    events = jsonl(os.path.join(d, "n1.stream")) or []
    res, init = result_of(events), init_of(events)
    reply = text_of(res)
    print("  nested run from the outer Stop hook: exit %d; result %s, is_error %s, num_turns %s; replied PONG %s; "
          "session id the one it was given %s" % (
              rc, closed(res.get("subtype"), SUBTYPES) if res else "none",
              "unknown" if not res or not isinstance(res.get("is_error"), bool) else yn(res["is_error"]),
              num(res.get("num_turns")) if res else "unknown",
              "unknown" if reply is None else yn(reply.strip() == "PONG"),
              "unknown" if not res else yn(res.get("session_id") == n1)))
    if init:
        # Whether MCP servers were listed, not how many: the number is the
        # account's configuration, and all this item needs is that --tools ""
        # left no tools with servers present.
        tools, mcp = init.get("tools"), init.get("mcp_servers")
        print("    its init event: tools %s, mcp servers listed %s" % (
            len(tools) if isinstance(tools, list) else "unknown", yn(mcp) if isinstance(mcp, list) else "unknown"))
    else:
        print("    its init event: not observed")
    recs = jsonl(os.path.join(d, "events.ndjson")) or []
    mine = collections.Counter(r.get("event") for r in recs if r.get("tag") == "nested" and r.get("session") == digest(n1))
    print("    hooks from its own --settings that fired: SessionStart %d, UserPromptSubmit %d, Stop %d" % (
        mine["SessionStart"], mine["UserPromptSubmit"], mine["Stop"]))
    window = []
    try:
        with open(os.path.join(d, "window"), encoding="utf-8") as f:
            window = [float(x) for x in f.read().split()]
    except (OSError, ValueError):
        pass
    if len(window) == 2:
        outer = sum(1 for r in recs if r.get("tag") == "outer" and r.get("event") != "Stop"
                    and isinstance(r.get("t"), (int, float)) and window[0] <= r["t"] <= window[1])
        print("    outer-session hooks other than its own Stop that fired while it ran: %d" % outer)
    else:
        print("    outer-session hooks while it ran: window not recorded")
    try:
        with open(os.path.join(d, "n1.debug"), encoding="utf-8", errors="replace") as f:
            lines = f.read().splitlines()
    except OSError:
        print("    its debug log: not written")
        return 0
    missing, other, dests, odest, remote = [], 0, set(), 0, False
    for ln in lines:
        m = re.search(r"missing file encountered for settings\.json at path: (.+)$", ln)
        if m:
            role = setting_role(m.group(1).strip(), cfg, cwd)
            if role is None:
                other += 1
            elif role not in missing:
                missing.append(role)
        m = re.search(r"rules for destination '([^']*)'", ln)
        if m:
            if m.group(1) in DESTINATIONS:
                dests.add(m.group(1))
            else:
                odest += 1
        remote = remote or "Remote settings:" in ln
    print("    settings files it looked for and found missing: %s; other paths %d" % (", ".join(missing) or "none", other))
    print("    settings destinations it applied rules for: %s; other %d" % (", ".join(sorted(dests)) or "none", odest))
    print("    remote settings consulted: " + yn(remote))
    return 0


def holding(cfg, since, marker):
    """Files under cfg changed since `since` whose bytes contain marker. Counted, never named."""
    n = 0
    needle = marker.encode("utf-8")
    for root, _dirs, files in os.walk(cfg):
        for name in files:
            p = os.path.join(root, name)
            try:
                st = os.stat(p)
                if st.st_mtime < since or st.st_size > 64 << 20:
                    continue
                with open(p, "rb") as f:
                    n += 1 if needle in f.read() else 0
            except OSError:
                continue
    return n


def hc6(d, cfg, n1, n2, since_file, marker1, marker2):
    """--no-session-persistence, with the prompt on stdin: no transcript, and no file holding the prompt."""
    rc = read_int(os.path.join(d, "n2.exit"))
    if rc is None:
        print("  nested run with the prompt on stdin and --no-session-persistence: did not run")
        return 0
    out = whole_json(os.path.join(d, "n2.json"))
    reply = text_of(out)
    print("  nested run with the prompt on stdin and --no-session-persistence: exit %d; is_error %s; replied PONG %s; "
          "session id the one it was given %s" % (
              rc, "unknown" if not out or not isinstance(out.get("is_error"), bool) else yn(out["is_error"]),
              "unknown" if reply is None else yn(reply.strip() == "PONG"),
              "unknown" if not out else yn(out.get("session_id") == n2)))
    projects = os.path.join(cfg, "projects")
    print("    transcripts named for it under the projects directory: %d; for the run before it, which kept its session: %d" % (
        len(glob.glob(os.path.join(projects, "*", n2 + ".jsonl"))), len(glob.glob(os.path.join(projects, "*", n1 + ".jsonl")))))
    try:
        since = os.stat(since_file).st_mtime
    except OSError:
        print("    files holding its prompt's marker: start of the check not recorded")
        return 0
    print("    files under the config directory changed since the check began holding its prompt's marker: %d; "
          "holding the earlier run's marker: %d" % (holding(cfg, since, marker2), holding(cfg, since, marker1)))
    return 0


def fm11(probe_log, store, sid):
    """What `rashomon probe start` and `probe end` printed under Claude Code, and that they ran."""
    rows = collections.defaultdict(list)
    try:
        with open(probe_log, encoding="utf-8") as f:
            for ln in f:
                parts = ln.split()
                if len(parts) == 5 and parts[1] in ("start", "end") and all(x.isdigit() for x in parts[2:]):
                    rows[parts[1]].append((parts[0] if RUN.match(parts[0]) else "other", int(parts[2]), int(parts[3]), int(parts[4])))
    except OSError:
        pass
    for phase in ("start", "end"):
        r = rows[phase]
        if not r:
            print("  probe %s: no invocations" % phase)
            continue
        print("  probe %s: invocations %d (runs %s), exit codes %s, stdout %d bytes, stderr %d bytes" % (
            phase, len(r), ", ".join(sorted({x[0] for x in r})), ", ".join(str(c) for c in sorted({x[1] for x in r})),
            sum(x[2] for x in r), sum(x[3] for x in r)))
    cov = collections.Counter(c.get("phase") for c in jsonl(os.path.join(store, "runs", sid, "coverage.ndjson")) or []
                              if c.get("type") == "coverage")
    print("  coverage records the probes wrote for this session: start %d, end %d" % (cov["start"], cov["end"]))
    return 0


def ptru1(stream, transcript, event, ctx, sm, fired):
    """P-TRU-1 on one event (test/live/p-tru-1.sh): one hook output carried ctx as
    additionalContext and sm as systemMessage. It passes when ctx is in a
    hook_additional_context attachment for the event in the session's
    transcript, what the agent is given, and sm in a system/informational event
    on the stream, what the user's terminal shows. 0 pass, 1 fail, 3 when the
    hook never ran, so the run says nothing of the channel."""
    events = jsonl(stream) or []
    if not os.path.exists(fired):
        print("  the hook ran: no")
        print("  verdict: inconclusive: no login, or the model did not run the command")
        return 3
    print("  the hook ran: yes")
    hooks(stream)
    recs = jsonl(transcript)
    seen = collections.Counter()
    for r in recs or []:
        a = r.get("attachment")
        if isinstance(a, dict) and a.get("type") in ATTACHMENTS and a.get("hookEvent") == event:
            blob = json.dumps(a)
            seen[(a["type"], "ctx")] += ctx in blob
            seen[(a["type"], "sm")] += sm in blob
    to_agent = seen[("hook_additional_context", "ctx")] > 0
    info = [e.get("content") for e in events if e.get("type") == "system" and e.get("subtype") == "informational"]
    to_user = sm in " ".join(c for c in info if isinstance(c, str))
    reply = text_of(result_of(events))
    print("  transcript %s; additionalContext in a hook_additional_context attachment for the event: %s" % (
        "found" if recs is not None else "not found", yn(to_agent)))
    print("  stream system/informational events: %d; systemMessage in one: %s" % (len(info), yn(to_user)))
    print("  outside the verdict: the reply names additionalContext %s and systemMessage %s; systemMessage in a "
          "hook_system_message attachment: %s" % (has(reply, ctx), has(reply, sm), yn(seen[("hook_system_message", "sm")] > 0)))
    ok = to_agent and to_user
    print("  verdict: " + ("pass" if ok else "fail"))
    return 0 if ok else 1


COMMANDS = {"record": (record, 2), "hooks": (hooks, 1), "hc1": (hc1, 6), "hc2": (hc2, 1), "hc3": (hc3, 1),
            "hc5": (hc5, 4), "hc6": (hc6, 7), "fm11": (fm11, 3), "fm12": (fm12, 1), "fm13": (fm13, 1),
            "ptru1": (ptru1, 6)}


def main(argv):
    if len(argv) < 2 or argv[1] not in COMMANDS or len(argv) - 2 != COMMANDS[argv[1]][1]:
        # Only ever reached by a mistake in hc-items.sh; a hook gets here
        # with the right arguments, so this costs it nothing.
        print("usage: hc_shape.py " + " | ".join(sorted(COMMANDS)), file=sys.stderr)
        return 0 if len(argv) > 1 and argv[1] == "record" else 2
    fn, _ = COMMANDS[argv[1]]
    return fn(*argv[2:])


if __name__ == "__main__":
    sys.exit(main(sys.argv))
