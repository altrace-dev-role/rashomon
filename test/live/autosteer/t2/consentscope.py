"""consentscope.py STORE: V1's read of one run's store (addendum 11 section 9, ruled 10:50 PST 11-14): after consent,
G's store reads record scope v5 with the session-content marker set. Three facts, each as G's binary reads it:

  fold     consent.SessionContent(STORE): the highest scope_version over the records that parse is at least
           disclose.SessionContentScope (6), and the latest record that names a record scope names v5.
  marker   store.SessionContentMarker(STORE): STORE/session_content holds "1" (surrounding white space aside).
  scope    store.RecordScopeMarker(STORE): STORE/record_scope holds "v5" (surrounding white space aside).

Exit 0 when all three hold, 1 when any does not (a refusal), 2 when consent.json exists and cannot be read, STORE is
not a directory, or the usage is wrong. One line on stdout names every fact read.

consent.json is read as consent.Read reads it: its first maxReadBytes, split on newlines, blank lines dropped, and a
line kept only when it decodes as encoding/json decodes into consent.Record (a JSON object or null; keys matched
exactly, else case-insensitively, the last occurrence winning, and a null leaving the field as it was; a typed field holding a value of another type, a
fraction or exponent in an integer, an integer out of int64, or NaN/Infinity fails the line) and Record.parses holds.
Invalid UTF-8 inside a string is replaced, as encoding/json does. Constants, at G's b43e7eb3 and cd82100a:
# src internal/consent/record.go:47 | const RecordVersion = 1
# src internal/consent/record.go:57 | const maxReadBytes = 4 << 20
# src internal/consent/record.go:78 | ViaWatchTTY       Via = "watch_tty"
# src internal/consent/record.go:85 | ViaSteerOn        Via = "steer_on"
# src internal/consent/units.go:18 | var Units = []string{"bodies", "share.outcomes", "share.lessons", "share.bodies"}
# src internal/disclose/sessioncontent.go:10 | const SessionContentScope = 6
# src internal/store/scope.go:24 | const FileRecordScope = "record_scope"
# src internal/store/scope.go:32 | RecordScopeV5 = "v5"
# src internal/store/sessioncontent.go:23 | const FileSessionContent = "session_content"
# src internal/store/store.go:45 | const FileConsent = "consent.json"
# src internal/consent/sessioncontent.go:37 | if !ok || best < disclose.SessionContentScope {
# src internal/consent/sessioncontent.go:42 | return s == RecordScopeV5, nil
"""
import json
import os
import sys

RECORD_VERSION = 1
MAX_READ_BYTES = 4 << 20
VIAS = ("watch_tty", "watch_yes", "plugin_enable", "first_run_notice", "upgrade_notice", "consent_tty",
        "consent_yes", "steer_on")
UNITS = ("bodies", "share.outcomes", "share.lessons", "share.bodies")
SESSION_CONTENT_SCOPE = 6
RECORD_SCOPE_V5 = "v5"
FILE_CONSENT = "consent.json"
FILE_SESSION_CONTENT = "session_content"
FILE_RECORD_SCOPE = "record_scope"
INT64 = (-(1 << 63), (1 << 63) - 1)

# consent.Record's fields by JSON name: int, str, bool, or optbool (*bool: a bool or null).
FIELDS = {
    "v": int, "scope_version": int, "record_scope": str, "steer_disclosure": int, "steer": str,
    "text_sha256": str, "binary_version": str, "binary_sha256": str, "via": str, "tty": bool,
    "plugin_option_steering": "optbool", "invocation_id": str, "at": int, "unit": str, "unit_on": bool,
}
ZERO = {"v": 0, "scope_version": 0, "record_scope": "", "steer_disclosure": 0, "via": "", "unit": ""}


class Pairs(list):
    """A JSON object as its (key, value) pairs in order, duplicates kept, as encoding/json walks it."""


class Bad(Exception):
    pass


def _const(name):
    raise Bad("JSON constant %s" % name)


def typed(want, v):
    if v is None:
        return True
    if want is int:
        return isinstance(v, int) and not isinstance(v, bool) and INT64[0] <= v <= INT64[1]
    if want is str:
        return isinstance(v, str)
    if want is bool or want == "optbool":
        return isinstance(v, bool)
    return False


def field_for(key):
    if key in FIELDS:
        return key
    k = key.casefold()
    for name in FIELDS:
        if name.casefold() == k:
            return name
    return None


def decode(line):
    """One consent.json line as Go decodes it into consent.Record: a dict of the fields read, or None when the line
    does not decode (encoding/json returns an error) or decodes to a record Record.parses rejects."""
    try:
        o = json.loads(line.decode("utf-8", "replace"), object_pairs_hook=Pairs, parse_constant=_const)
    except (ValueError, Bad, RecursionError):
        return None
    rec = dict(ZERO)
    if o is not None:
        if not isinstance(o, Pairs):
            return None
        for key, v in o:
            name = field_for(key)
            if name is None:
                continue
            if not typed(FIELDS[name], v):
                return None
            # encoding/json: a null into a non-pointer field "has no effect on the value", so an earlier duplicate stays.
            if name in rec and v is not None:
                rec[name] = v
    if rec["v"] != RECORD_VERSION or rec["via"] not in VIAS:
        return None
    if rec["unit"] != "" and not (rec["unit"] in UNITS and rec["scope_version"] == 0 and rec["record_scope"] == ""
                                  and rec["steer_disclosure"] == 0):
        return None
    return rec


def read_records(store):
    """consent.Read: (records, lines skipped); a missing file is no records. Raises OSError for any other failure."""
    try:
        with open(os.path.join(store, FILE_CONSENT), "rb") as f:
            data = f.read(MAX_READ_BYTES)
    except FileNotFoundError:
        return [], 0
    recs, skipped = [], 0
    for line in data.split(b"\n"):
        if not line.strip():
            continue
        r = decode(line)
        if r is None:
            skipped += 1
        else:
            recs.append(r)
    return recs, skipped


def marker(store, name):
    """The marker's trimmed text, or None when it cannot be read (os.ReadFile's error: the reader's false or v4)."""
    try:
        with open(os.path.join(store, name), "rb") as f:
            return f.read().decode("utf-8", "replace").strip()
    except OSError:
        return None


def check(store):
    if not os.path.isdir(store):
        return 2, "STORE [%s] is not a directory" % store
    try:
        recs, skipped = read_records(store)
    except OSError as e:
        return 2, "%s/%s cannot be read: %s" % (store, FILE_CONSENT, e)
    best = max((r["scope_version"] for r in recs), default=None)
    latest = next((r["record_scope"] for r in reversed(recs) if r["record_scope"] != ""), None)
    fold = best is not None and best >= SESSION_CONTENT_SCOPE and latest == RECORD_SCOPE_V5
    sc = marker(store, FILE_SESSION_CONTENT)
    rs = marker(store, FILE_RECORD_SCOPE)
    ok = fold and sc == "1" and rs == RECORD_SCOPE_V5
    why = ("records %d (%d lines skipped); highest scope_version %s (want %d or more); latest record scope %s "
           "(want v5); fold %s; session_content marker %s (want 1); record_scope marker %s (want v5)"
           % (len(recs), skipped, best, SESSION_CONTENT_SCOPE, latest, "yes" if fold else "no",
              "absent" if sc is None else repr(sc), "absent" if rs is None else repr(rs)))
    return (0 if ok else 1), why


def main(argv):
    if len(argv) != 1:
        print("consentscope: usage: consentscope.py STORE", file=sys.stderr)
        return 2
    rc, why = check(argv[0])
    print("consentscope %s: %s: %s" % (argv[0], {0: "PASS", 1: "REFUSE"}.get(rc, "UNREADABLE"), why))
    return rc


if __name__ == "__main__":
    try:
        code = main(sys.argv[1:])
    except Exception as e:
        print("consentscope: %s: %s" % (type(e).__name__, e), file=sys.stderr)
        code = 2
    sys.exit(code)
