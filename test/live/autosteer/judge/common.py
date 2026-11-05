"""Shared parsers for the AutoSteer live judges. Nothing here prints transcript content.

A run directory (test/live/autosteer/run.sh) holds transcript.jsonl (Claude Code's own), result.json,
hooks.log (R and G), status.txt, diff.patch, new-files.txt, report.txt, arm, exit.
"""
import json, os, re

EDIT_TOOLS = ("Edit", "Write", "MultiEdit", "NotebookEdit")


def load_jsonl(path):
    out = []
    if not os.path.exists(path):
        return out
    for line in open(path, encoding="utf-8", errors="replace"):
        line = line.strip()
        if not line:
            continue
        try:
            out.append(json.loads(line))
        except ValueError:
            continue
    return out


def tool_calls(transcript):
    """Every tool_use in order: (index, name, input dict)."""
    calls = []
    for rec in transcript:
        if rec.get("type") != "assistant":
            continue
        for c in rec.get("message", {}).get("content", []) or []:
            if isinstance(c, dict) and c.get("type") == "tool_use":
                calls.append((len(calls), c.get("name", ""), c.get("input") or {}))
    return calls


def first_edit_index(calls):
    for i, name, _ in calls:
        if name in EDIT_TOOLS:
            return i
    return None


def texts_before_first_edit(transcript):
    """The assistant's own text blocks before its first edit tool call (used only for name matching)."""
    out = []
    for rec in transcript:
        if rec.get("type") != "assistant":
            continue
        for c in rec.get("message", {}).get("content", []) or []:
            if not isinstance(c, dict):
                continue
            if c.get("type") == "tool_use" and c.get("name") in EDIT_TOOLS:
                return out
            if c.get("type") == "text":
                out.append(c.get("text", ""))
    return out


def call_mentions(name, inp, rel):
    """Whether one tool call opened or searched a repo-relative path. Read: the path is the file; Grep, Glob,
    LS: the path or pattern is the file or its directory; Bash: the command names the file or its directory."""
    d = os.path.dirname(rel)
    if name == "Read":
        return "opened" if str(inp.get("file_path", "")).endswith(rel) else None
    if name in ("Grep", "Glob", "LS"):
        hay = " ".join(str(inp.get(k, "")) for k in ("path", "pattern", "glob", "include"))
        return "searched" if (rel in hay or (d and hay.rstrip("/").endswith(d))) else None
    if name == "Bash":
        cmd = str(inp.get("command", ""))
        if rel in cmd:
            return "opened" if re.search(r"\b(cat|sed|head|tail|less|grep|rg|awk)\b", cmd) else "searched"
        if d and (d + "/") in cmd or (d and cmd.rstrip().endswith(d)):
            return "searched"
    return None


def result_json(path):
    try:
        r = json.load(open(path))
    except (OSError, ValueError):
        return {}
    if isinstance(r, list):
        r = next((x for x in r if isinstance(x, dict) and x.get("type") == "result"), r[-1] if r else {})
    return r if isinstance(r, dict) else {}


def hook_events(path):
    """hooks.log blocks: [{verb, args, rc, stdout}] in order."""
    events, cur = [], None
    if not os.path.exists(path):
        return events
    for line in open(path, encoding="utf-8", errors="replace"):
        m = re.match(r"^== (\d\d:\d\d:\d\dZ) (\S+)(.*) rc=(\d+)$", line.rstrip("\n"))
        if m:
            cur = {"at": m.group(1), "verb": m.group(2), "args": m.group(3).strip(), "rc": int(m.group(4)), "stdout": ""}
            events.append(cur)
        elif cur is not None:
            cur["stdout"] += line
    return events


def additional_context(stdout):
    """Every additionalContext and systemMessage string in a hook's stdout JSON objects."""
    out = []
    for m in re.finditer(r"\{.*\}", stdout, re.S):
        try:
            o = json.loads(m.group(0))
        except ValueError:
            continue
        h = o.get("hookSpecificOutput") or {}
        for k in ("additionalContext",):
            if isinstance(h.get(k), str):
                out.append(h[k])
        if isinstance(o.get("systemMessage"), str):
            out.append(o["systemMessage"])
    return out


def ints_on_line(line):
    return [int(x) for x in re.findall(r"(?<![\w.])\d+(?![\w.])", line)]
