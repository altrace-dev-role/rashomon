# STR-9: the mirror is printed byte for byte. encoding/json prints invalid
# UTF-8 as U+FFFD, so Marshal refuses such a string; dropping that check lets
# the printed bytes differ from the logged ones, which is the regression.
m("STR-9 Marshal lets encoding/json rewrite invalid UTF-8", "internal/steer/finding.go",
  "\tif err := o.checkUTF8(); err != nil {\n\t\treturn nil, err\n\t}\n",
  "", "TestSTR9b_")
