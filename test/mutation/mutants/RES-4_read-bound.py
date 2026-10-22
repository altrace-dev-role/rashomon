# RES-4: hookio.Read's growth is bounded, its cap is exact at N and N+1, and
# a refusal is recorded as payload_too_large. Reading through io.ReadAll's
# doubling, moving the cap by one byte, or folding the refusal into
# internal_error is each the whole regression.
m("RES-4 the payload is read with io.ReadAll's doubling", "internal/hookio/read.go",
  "\tvar chunks [][]byte\n",
  "\tif b, err := io.ReadAll(io.LimitReader(r, MaxPayloadBytes+1)); err != nil || len(b) <= MaxPayloadBytes {\n\t\treturn b, nil, err\n\t} else {\n\t\treturn nil, b[:firstChunk], ErrTooLarge\n\t}\n\tvar chunks [][]byte\n",
  "TestRES4_")
m("RES-4 a payload of exactly the cap is refused", "internal/hookio/read.go",
  "\t\tif total > MaxPayloadBytes {\n", "\t\tif total >= MaxPayloadBytes {\n", "TestRES4_")
m("RES-4 the refusal is recorded as an internal error", "internal/hook/payload.go",
  "\tcase errors.Is(err, hookio.ErrTooLarge):\n\t\treturn store.ReasonPayloadTooLarge\n",
  "\tcase errors.Is(err, hookio.ErrTooLarge):\n\t\treturn store.ReasonInternalError\n", "TestRES4_")

# The *bytes.Reader fast path (internal/hostguard's own second read of an
# already-read payload, Technical spec 6.2.3) keeps the same bound and the
# same refusal RES-4 pins above, just by a different route: Len() instead
# of the bounded growth loop.
m("RES-4 the bytes.Reader fast path refuses one byte too late", "internal/hookio/read.go",
  "\t\tif n > MaxPayloadBytes {\n\t\t\thead = make([]byte, firstChunk)",
  "\t\tif n > MaxPayloadBytes+1 {\n\t\t\thead = make([]byte, firstChunk)",
  "TestRead_BytesReaderFastPath")
m("RES-4 the bytes.Reader fast path returns an unread buffer", "internal/hookio/read.go",
  "\t\tout = make([]byte, n)\n\t\tif _, err := io.ReadFull(br, out); err != nil {\n\t\t\treturn nil, nil, err\n\t\t}\n\t\treturn out, nil, nil",
  "\t\tout = make([]byte, n)\n\t\treturn out, nil, nil",
  "TestRead_BytesReaderFastPath")
# Over the cap, the fast path still reads the first chunk, the head an
# over-cap refusal is attributed to its session from.
m("RES-4 the bytes.Reader fast path refuses with no head to attribute", "internal/hookio/read.go",
  "\t\t\treturn nil, head, ErrTooLarge\n\t\t}\n\t\tout = make([]byte, n)",
  "\t\t\treturn nil, nil, ErrTooLarge\n\t\t}\n\t\tout = make([]byte, n)",
  "TestRES4_")
