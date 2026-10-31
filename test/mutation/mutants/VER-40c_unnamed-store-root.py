# VER-40c: the store-root bit is read wherever integrity counts a call
# unnamed (D173c): an MCP call, a file tool's write with no file_path, a shell
# call whose written list is incomplete, a may-write with no list, at record
# scope v5 as at v4. One break per case, and one for the read itself. The kill
# rows are TestST1x_ (the bit decides at v5 and v4) and TestST1y_ (the helper
# and appendWrites' unnamed list agree).
I = "internal/verify/integrity.go"

m("VER-40c the bit is not read for an unnamed call", I,
  "\tif !hit && d.StoreNamed && writesUnnamed(d) {\n", "\tif false && !hit && d.StoreNamed && writesUnnamed(d) {\n", "TestST1x_")
m("VER-40c a call that shows the root counts twice", I,
  "\tif !hit && d.StoreNamed && writesUnnamed(d) {\n", "\tif d.StoreNamed && writesUnnamed(d) {\n", "TestST1x_")
m("VER-40c an MCP call is not unnamed", I,
  "\tcase d.Shape.VerbClass == shape.VerbMCP:\n\t\treturn true\n", "\tcase d.Shape.VerbClass == shape.VerbMCP:\n\t\treturn false\n",
  "TestST1x_|TestST1y_")
m("VER-40c a write with no file_path is not unnamed", I, "\t\treturn file == nil\n", "\t\treturn file != nil\n", "TestST1x_|TestST1y_")
m("VER-40c an explicit incomplete list is read as complete", I,
  "\t\treturn d.WrittenPathsComplete == nil || !*d.WrittenPathsComplete\n", "\t\treturn d.WrittenPathsComplete == nil\n",
  "TestST1x_|TestST1y_")
m("VER-40c a list that is not complete is not unnamed", I,
  "\t\treturn d.WrittenPathsComplete == nil || !*d.WrittenPathsComplete\n", "\t\treturn false\n", "TestST1x_|TestST1y_")
m("VER-40c a may-write with no list is not unnamed", I, "\treturn d.Shape.MayWrite\n}", "\treturn false\n}", "TestST1x_|TestST1y_")
