m("an empty run directory keeps the zero mtime instead of its own", "internal/store/evict.go",
  "\tif len(files) == 0 {\n\t\tinfo, err := os.Stat(dir)\n\t\tif err != nil {\n\t\t\treturn c, err\n\t\t}\n\t\tc.mtime = info.ModTime()\n\t}\n",
  "",
  "TestEvict_EmptyRunDirIsNotTreatedAsTheOldest")
