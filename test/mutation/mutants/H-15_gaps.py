m("H-15 forget deletes without a gap record", "internal/store/gaps.go", "\tg.RemovedRecords = removedRec + removedSpill\n\tif err := s.AppendGap(g); err != nil {\n\t\treturn nil, err\n\t}\n", "\tg.RemovedRecords = removedRec + removedSpill\n", "TestH15_ForgetLeavesAGap")
m("H-15 eviction deletes without a gap record", "internal/store/evict.go", "\t\tif err := appendHealed(gf, line, true); err != nil {", "\t\tif err := appendHealed(gf, line[:0], true); err != nil {", "TestH15_SizeCap")
m("H-15 eviction does not count the executions it removed", "internal/store/evict.go",
  "g.RemovedRecords = len(run.Declarations) + len(run.Executions) + len(run.Terminals)\n",
  "g.RemovedRecords = len(run.Declarations) + len(run.Terminals)\n", "TestH15_SizeCap")
m("H-15 forget --before behaves like --since", "cmd/rashomon/main.go",
  "\t\t\tif flag == \"--since\" {\n\t\t\t\tfrom = &t\n\t\t\t} else {\n\t\t\t\tto = &t\n\t\t\t}", "\t\t\tfrom = &t",
  "TestH15_Forget")
m("H-15 forget takes both windows and resolves them itself", "cmd/rashomon/main.go",
  "\tcase from != nil && to != nil:\n\t\treturn errors.New(\"--since and --before name opposite ends of the store's timeline; pass one or the other\")\n",
  "", "TestH15_ForgetRefusesBothWindows")
m("H-15 executions are not part of forget's doomed plan", "internal/store/gaps.go",
  "\t\tvar h lineHead\n\t\tif json.Unmarshal(line, &h) != nil {\n\t\t\tcontinue\n\t\t}",
  "\t\tvar h lineHead\n\t\tif json.Unmarshal(line, &h) != nil || bytes.Contains(line, []byte(`\"type\":\"execution\"`)) {\n\t\t\tcontinue\n\t\t}",
  "TestH15_ForgetTakesTheExecutionWithThePair")
IMPORTS["H-15 executions are not part of forget's doomed plan"] = ("internal/store/gaps.go", '\t"crypto/hmac"\n', '\t"bytes"\n\t"crypto/hmac"\n')
m("H-15 forget --before writes no gap record", "internal/store/gaps.go",
  "\tg.RemovedRecords = removedRec + removedSpill\n\tif err := s.AppendGap(g); err != nil {\n\t\treturn nil, err\n\t}",
  "\tg.RemovedRecords = removedRec + removedSpill\n\tif !w.openBelow() {\n\t\tif err := s.AppendGap(g); err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t}",
  "TestH15_Forget")
m("H-15 the gap's open end is closed by the bound nobody named", "internal/store/gaps.go",
  "\tif w.openBelow() {\n\t\tfrom = earliest\n\t}", "", "TestH15_ForgetBeforeLeavesAGap")
