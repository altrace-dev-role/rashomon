# RES-6: a large field is read through one work buffer, never as a whole
# string. Unescaping the whole span first is the regression.
m("RES-6 Lines unescapes the whole span into one slice", "internal/jsonscan/lines.go",
  "\tfor r := 0; r < len(b) && !l.stopped; {\n\t\tseg, next := segment(b, r, &tmp)\n\t\tr = next\n\t\tl.feed(seg, true)\n\t}\n",
  "\twhole, _ := unescapeHead(b, tmp[:0], 3*len(b))\n\tl.feed(whole, true)\n",
  "TestRES6_")
