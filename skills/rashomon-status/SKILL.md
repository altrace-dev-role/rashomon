---
name: rashomon-status
description: 'Say what rashomon has installed here: hook entries, store location, install ids, whether hooks are disabled. Use when the user says "rashomon status", "is rashomon running", "is rashomon recording", "is the recorder on".'
---

1. Resolve the binary: `~/.local/bin/rashomon`, then `~/go/bin/rashomon`,
   then `command -v rashomon`. This skill answers a question — if no binary
   exists, the answer is "rashomon is not installed on this machine"; say
   that and point at `/rashomon`. Never build or install anything from here.

2. Run `$RASHOMON status` and show the output as-is. It reads; it writes
   nothing and creates no store. Entries read `present`, `absent`,
   `unreadable`, or `unknown` — and on a machine with no store, every event
   reads `unknown`, because with no install id nothing can be called ours.
   That is the correct answer, not a failure.

3. If entries read `absent` and the user wanted recording, point them at
   `/rashomon`. If another install's entries share the file, relay that
   line — those entries fire here and record nothing in this store.

4. The `recap:` line says whether the exception-only line (Stop/StopFailure)
   has ever actually run: `not evaluated` means no turn has completed since
   install, `evaluated` means one has, whether or not it had anything to say
   — silence on a turn is never proof it was clean, only that recap had
   nothing exception-worthy to report or was never asked. If it says a run
   "did not complete cleanly", that failure is recap's own and not the
   session's; point the user at `/rashomon-report` for the session itself.
