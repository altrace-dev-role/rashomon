# RES-4: a refused payload is recorded in the run its first 4 KiB name.
# Dropping the head, or attributing from a head cut before its end, is the
# regression.
m("RES-4 a refusal drops the head that names its session", "internal/hookio/payload.go",
  "\t\treturn nil, refused(head)\n",
  "\t\t_ = head\n\t\treturn nil, &TooLargeError{}\n",
  "TestRES4_")
m("RES-4 a head cut short of its end still names a session", "internal/hookio/payload.go",
  "\tif !errors.As(err, &se) || se.Offset != int64(len(head)) {\n",
  "\tif !errors.As(err, &se) {\n",
  "TestRES4_")
m("RES-4 a refusal is not ErrTooLarge", "internal/hookio/payload.go",
  "func (e *TooLargeError) Is(target error) bool { return target == ErrTooLarge }\n",
  "func (e *TooLargeError) Is(target error) bool { return false }\n",
  "TestRES4_")
m("RES-4 Read refuses without the head that names the session", "internal/hookio/read.go",
  "\t\treturn nil, refused(head)\n",
  "\t\t_ = head\n\t\treturn nil, ErrTooLarge\n",
  "TestRES4_")
m("RES-4 Intake refuses a replayed refusal afresh from an empty head", "internal/hookio/payload.go",
  "\tif errors.As(err, &tl) {\n\t\treturn nil, tl\n\t}\n",
  "\tif false && errors.As(err, &tl) {\n\t\treturn nil, tl\n\t}\n",
  "TestRES4_")
