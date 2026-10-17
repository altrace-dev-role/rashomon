# RES-5: a hook payload is validated and scanned once. A second decode of the
# raw payload on a hook path, or a walker that decides validity itself, is
# the regression.
m("RES-5 post decodes the payload again for the background keys", "internal/hook/post.go",
  "\tvar bits [2]jsonscan.Span\n",
  "\tvar again map[string]json.RawMessage\n\tif json.Unmarshal(in.Raw, &again) != nil {\n\t\treturn false\n\t}\n\tvar bits [2]jsonscan.Span\n",
  "TestRES5_")
m("RES-5 Top walks without json.Valid's verdict", "internal/jsonscan/jsonscan.go",
  "\tif !json.Valid(buf) {\n\t\treturn ErrInvalid\n\t}\n",
  "\tif !json.Valid(buf[:0]) && false {\n\t\treturn ErrInvalid\n\t}\n",
  "TestRES5_")
