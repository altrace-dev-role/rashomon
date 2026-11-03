# SPN-1 (#104): a spill made without its lock leads with a newline so it cannot glue onto a torn tail, and every
# reader of the spill file skips the blank line that leaves.
m("SPN-1 the lockless spill glues onto a torn tail", "internal/store/store.go",
  "\t\t_, err = f.Write(append([]byte{'\\n'}, line...))\n", "\t\t_, err = f.Write(line)\n",
  "TestSPN1_ALocklessSpillDoesNotGlueOntoATornTail")
m("SPN-1 a blank line reads as a line over the limit", "internal/store/lines.go",
  "\t\tcase len(line) > 0:\n\t\t\tkeep = fn(line)\n", "\t\tdefault:\n\t\t\tkeep = fn(line)\n",
  "TestSPN1_ALocklessSpillReadsBack")
