# B9 -- naming the rewritten calls. The wording split is the correctness half:
# shape.Derive digests the whole input for every tool but Bash, so calling a
# non-Bash difference a changed COMMAND is false.
m("B9 every rewritten call is described as a command", "internal/report/text.go",
  '\t\twhat := "input changed"\n\t\tif r.ToolName == "Bash" {\n\t\t\twhat = "command changed"\n\t\t}',
  '\t\twhat := "command changed"', "TestRewritten")
m("B9 the count and the rows are computed separately", "internal/report/destinations.go",
  "\treturn len(rewrittenCalls(run))", "\treturn len(rewrittenCalls(run)) + 1", "TestRewritten|TestWireOnly")
