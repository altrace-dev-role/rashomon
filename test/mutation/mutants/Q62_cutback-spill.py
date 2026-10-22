m("a failed spill write is cut back like a locked one", "internal/store/tail.go",
  "func healAppendError(f *os.File, size int64, n int, cut bool, writeErr error) error {\n\tif !cut {\n\t\treturn writeErr\n\t}\n\treturn errors.Join(writeErr, cutBack(f, size, n))\n}",
  "func healAppendError(f *os.File, size int64, n int, cut bool, writeErr error) error {\n\treturn errors.Join(writeErr, cutBack(f, size, n))\n}",
  "TestHealAppendErrorCutFalseLeavesTheWriteOnDisk")
m("a failed locked write is never cut back", "internal/store/tail.go",
  "func healAppendError(f *os.File, size int64, n int, cut bool, writeErr error) error {\n\tif !cut {\n\t\treturn writeErr\n\t}\n\treturn errors.Join(writeErr, cutBack(f, size, n))\n}",
  "func healAppendError(f *os.File, size int64, n int, cut bool, writeErr error) error {\n\tif cut {\n\t\treturn writeErr\n\t}\n\treturn errors.Join(writeErr, cutBack(f, size, n))\n}",
  "TestHealAppendErrorCutTrueTruncates")
