m("eviction keeps removing runs past its caller's deadline", "internal/store/evict.go",
  "\t\t// A probe's deadline is shared across every lock wait it makes\n\t\t// (LimitLockWaits); eviction runs inside that same hook invocation.\n\t\t// A deep backlog must not make one pass run past it looking for\n\t\t// more to remove, where the hook's own timeout could cut a removal\n\t\t// short mid-delete. Checked between runs, never inside one, so a\n\t\t// run this pass starts removing always finishes. What is left\n\t\t// waits for the next probe.\n\t\tif !s.waitUntil.IsZero() && time.Now().After(s.waitUntil) {\n\t\t\tbreak\n\t\t}\n",
  '',
  "TestEvictStopsAtItsCallersDeadline")
