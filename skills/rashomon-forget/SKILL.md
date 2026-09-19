---
name: rashomon-forget
description: 'Erase recorded activity: evict every call naming a host, or everything recorded in a time window, leaving a gap record behind. Use when the user says "rashomon forget", "erase that host", "delete what you recorded", "remove the last hour", "I did not mean to record that".'
---

Capture was one word away and erasure was not reachable at all. That asymmetry
is the wrong way round for a recorder, so this skill exists to make the privacy
action as easy as the install.

1. Resolve the binary: `~/.local/bin/rashomon`, then `~/go/bin/rashomon`, then
   `command -v rashomon`. If no binary exists there is nothing to forget from
   — say so; never build or install from here.

2. ASK WHICH SHAPE, and do not guess. The three forms answer different
   questions and are not interchangeable:
   - `$RASHOMON forget --host <hostname>` — every call that NAMED that host,
     and the host's entry in the project baseline. Use when the concern is a
     destination: an internal service, a customer's domain.
   - `$RASHOMON forget --since <time>` — everything recorded at or after a
     point. Use for "I did not mean to record the last hour". Takes an
     RFC 3339 time or a duration such as `24h`, meaning that long ago.
   - `$RASHOMON forget --before <time>` — the retention counterpart:
     everything older than a point.

   `--since` and `--before` are opposite open ends of one window and naming
   both is refused rather than resolved, so ask for one.

3. Run it and show the output verbatim, including the count of records
   evicted. Do not summarise it as "done": the user asked for erasure and the
   number is the evidence.

4. Say what it did NOT do, every time, because the alternative is a user who
   believes more was erased than was:
   - A GAP RECORD IS LEFT BEHIND. It says a forget happened, when, and how
     many records went — never what they were. The report renders it, so a
     later reader can tell "nothing happened here" from "something was
     removed here". That is deliberate: a store that could be silently
     emptied would be worthless as evidence, including to the user.
   - `--host` records the host as a KEYED DIGEST in that gap, not as a name,
     so the hostname does not survive in the bytes.
   - IT DOES NOT TOUCH THE PROXY'S OWN STORE. That database belongs to a
     different program, is opened read-only, and is hash-chained — deleting
     a row would break the chain it exists to provide. The report suppresses
     the destination from its view instead, which is a different thing from
     the row being gone, and the user should hear the difference.
   - Nothing is erased from Claude Code's own transcripts. They are not this
     program's files.

5. If the user wanted everything gone, that is not this command: the store is
   a directory, and removing it is theirs to do. Tell them where it is
   (`$RASHOMON status` prints the path) rather than running `rm` for them.

6. Forgetting what was never recorded is a satisfied request, not an error —
   if the output says nothing was recorded here, report that plainly rather
   than treating it as a failure.
