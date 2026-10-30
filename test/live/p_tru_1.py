#!/usr/bin/env python3
"""P-TRU-1's reader: did a probe hook's text reach the model, per event?

test/live/p-tru-1.sh runs Claude Code sessions whose PostToolUse and
PostToolUseFailure hooks each emit, for the event, a context token in
hookSpecificOutput.additionalContext and a mirror token in systemMessage, both
carrying the run's nonce. This file reads what a session left -- its
transcript, and for a claude -p run its stream-json output -- and writes the
artifact steer/client.go's verified list is extended from.

    settings F
        F is the scratch config's settings.json. Prints its key names as a
        JSON list, or null when there is none; refuses (exit 1) a file that
        is not a JSON object or that holds hooks or an enabled plugin, whose
        hooks would fire beside the probe's.
    report [--stream F] --transcript F --nonce N --settings-keys K --out F
        The version is the stream's init event's; without a stream, the one
        version the transcript records, else null.

Per event it reads, from the transcript:
  fired      an attachment entry names the event as its hookEvent
  delivered  a hook_additional_context attachment for the event holds the
             context token in what the client rendered for the model
  echoed     an assistant message repeats the context token: the model saw it
  mirror     a hook_system_message attachment for the event holds the mirror
             token, the line the user is shown
and whether the mirror token reached the model, which it never should.
PostToolUse is the control: it is verified on 2.1.280, so a run in which its
context is not delivered and echoed proves nothing about the other event, and
report exits 2.

Tokens of another nonce are not this run's and count for nothing. Only key
names, booleans, the version, the entrypoint, paths and digests are written;
no message text, no settings value. Run it with python3 -I.
"""
import argparse
import datetime
import hashlib
import json
import os
import sys

EVENTS = ("PostToolUse", "PostToolUseFailure")
CONTROL = "PostToolUse"


def context_token(event, nonce):
    return f"PTRU-CONTEXT-{event}-{nonce}"


def mirror_token(event, nonce):
    return f"PTRU-MIRROR-{event}-{nonce}"


def entries(path):
    """The transcript's JSON lines; a line that is not JSON is skipped."""
    with open(path, encoding="utf-8", errors="replace") as f:
        for line in f:
            try:
                e = json.loads(line)
            except ValueError:
                continue
            if isinstance(e, dict):
                yield e


def texts(v):
    """Every string inside v, however nested."""
    if isinstance(v, str):
        yield v
    elif isinstance(v, list):
        for x in v:
            yield from texts(x)
    elif isinstance(v, dict):
        for x in v.values():
            yield from texts(x)


def holds(v, token):
    return any(token in s for s in texts(v))


def assistant_text(e):
    """The text blocks of an assistant message entry, else nothing."""
    if e.get("type") != "assistant":
        return []
    content = (e.get("message") or {}).get("content")
    if isinstance(content, str):
        return [content]
    return [b.get("text", "") for b in content or [] if isinstance(b, dict) and b.get("type") == "text"]


def init_version(stream):
    """claude_code_version from the stream's system init event, else None."""
    for e in entries(stream):
        if e.get("type") == "system" and e.get("subtype") == "init":
            v = e.get("claude_code_version")
            return v if isinstance(v, str) else None
    return None


def settings(args):
    if not os.path.exists(args.path):
        print("null")
        return 0
    try:
        with open(args.path, encoding="utf-8") as f:
            s = json.load(f)
    except (OSError, ValueError) as e:
        print(f"refusing: {args.path} is not readable JSON: {e}", file=sys.stderr)
        return 1
    if not isinstance(s, dict):
        print(f"refusing: {args.path} is not a JSON object", file=sys.stderr)
        return 1
    plugins = s.get("enabledPlugins")
    enabled = any(plugins.values()) if isinstance(plugins, dict) else bool(plugins)
    if s.get("hooks") or enabled:
        print(f"refusing: {args.path} holds hooks or an enabled plugin, which would fire beside the probe's",
              file=sys.stderr)
        return 1
    print(json.dumps(sorted(s)))
    return 0


def read(transcript, nonce):
    rows = {ev: {"event": ev, "fired": False, "delivered": False, "echoed": False, "mirror": False,
                 "mirror_reached_model": False} for ev in EVENTS}
    versions, entrypoints = set(), set()
    for e in entries(transcript):
        if isinstance(e.get("version"), str):
            versions.add(e["version"])
        if isinstance(e.get("entrypoint"), str):
            entrypoints.add(e["entrypoint"])
        said = assistant_text(e)
        for ev in EVENTS:
            r = rows[ev]
            if any(context_token(ev, nonce) in s for s in said):
                r["echoed"] = True
            if any(mirror_token(ev, nonce) in s for s in said):
                r["mirror_reached_model"] = True
        a = e.get("attachment")
        if e.get("type") != "attachment" or not isinstance(a, dict) or a.get("hookEvent") not in rows:
            continue
        r = rows[a["hookEvent"]]
        r["fired"] = True
        kind = a.get("type")
        if kind == "hook_additional_context" and holds(e.get("rendered"), context_token(r["event"], nonce)):
            r["delivered"] = True
        if kind == "hook_system_message" and holds(a.get("content"), mirror_token(r["event"], nonce)):
            r["mirror"] = True
        if kind != "hook_system_message" and holds(e.get("rendered"), mirror_token(r["event"], nonce)):
            r["mirror_reached_model"] = True
    return [rows[ev] for ev in EVENTS], sorted(versions), sorted(entrypoints)


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 16), b""):
            h.update(chunk)
    return h.hexdigest()


def report(args):
    keys = json.loads(args.settings_keys)
    if keys is not None and not (isinstance(keys, list) and all(isinstance(k, str) for k in keys)):
        print("p_tru_1.py: --settings-keys is null or a JSON list of names", file=sys.stderr)
        return 1
    events, versions, entrypoints = read(args.transcript, args.nonce)
    control = next(r for r in events if r["event"] == CONTROL)
    if args.stream:
        version, source = init_version(args.stream), "init"
    else:
        version, source = (versions[0], "transcript") if len(versions) == 1 else (None, None)
    art = {
        "probe": "P-TRU-1",
        "run_at": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "version": version,
        "version_source": source,
        "transcript_versions": versions,
        "entrypoints": entrypoints,
        "settings_keys": keys,
        "nonce": args.nonce,
        "transcript": args.transcript,
        "transcript_sha256": sha256(args.transcript),
        "control_ok": control["delivered"] and control["echoed"],
        "events": events,
    }
    with open(args.out, "x", encoding="utf-8") as f:
        json.dump(art, f, indent=1, sort_keys=True)
        f.write("\n")
    print(f"version: {art['version']} from {source} (transcript: {', '.join(versions) or 'none'}; "
          f"entrypoint: {', '.join(entrypoints) or 'none'})")
    for r in events:
        print(f"{r['event']}: fired {r['fired']}, delivered {r['delivered']}, echoed {r['echoed']}, "
              f"mirror {r['mirror']}, mirror reached the model {r['mirror_reached_model']}")
    print(f"control ({CONTROL} delivered and echoed): {art['control_ok']}")
    print(f"artifact: {args.out} sha256 {sha256(args.out)}")
    print(f"transcript: {args.transcript} sha256 {art['transcript_sha256']}")
    return 0 if art["control_ok"] else 2


def main():
    p = argparse.ArgumentParser(prog="p_tru_1.py")
    sub = p.add_subparsers(dest="cmd", required=True)
    st = sub.add_parser("settings")
    st.add_argument("path")
    r = sub.add_parser("report")
    r.add_argument("--stream")
    r.add_argument("--transcript", required=True)
    r.add_argument("--nonce", required=True)
    r.add_argument("--settings-keys", required=True)
    r.add_argument("--out", required=True)
    args = p.parse_args()
    if args.cmd == "settings":
        return settings(args)
    if not args.nonce.isalnum():
        print("p_tru_1.py: the nonce is letters and digits only", file=sys.stderr)
        return 1
    return report(args)


if __name__ == "__main__":
    sys.exit(main())
