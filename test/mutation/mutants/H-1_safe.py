m("H-1  Guard no longer recovers", "internal/safe/safe.go",
  "\tdefer func() {\n\t\tif v := recover(); v != nil {\n\t\t\terr = newPanicError(v)\n\t\t}\n\t}()\n\treturn fn()", "\treturn fn()", "TestH1_NoFaultBlocksTheToolCall")
m("H-1  Go no longer recovers inside the goroutine", "internal/safe/safe.go",
  "\t\tdefer func() {\n\t\t\tif v := recover(); v != nil && onPanic != nil {\n\t\t\t\tdefer func() { _ = recover() }()\n\t\t\t\tonPanic(newPanicError(v))\n\t\t\t}\n\t\t}()\n\t\tfn()", "\t\tfn()", "TestH1_GoroutinePanicIsContained")
