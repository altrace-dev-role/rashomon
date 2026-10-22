# perf-6: readChunk is where a store read starts, not a size every record
# fits; a record over it is assembled from more chunks (rubric fix list entry
# 68).
m("perf-6  a store read stops at the initial buffer", "internal/store/lines.go",
  "\t\t\tif len(line)+len(chunk) > maxLine {",
  "\t\t\tif len(line)+len(chunk) > readChunk {", "TestRubricPERF6_")
