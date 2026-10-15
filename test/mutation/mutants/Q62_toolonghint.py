m("an overlong line's forget error drops the file name and the Evict hint", "internal/store/gaps.go",
  "\treturn fmt.Errorf(\"%s: %w; Evict still removes the whole run without reading it\", file, err)\n",
  "\treturn err\n",
  "TestForgetWindow_NamesTheFileAndTheRemedyForAnOverlongLine")
