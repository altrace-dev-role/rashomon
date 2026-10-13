# RES-4: hookio.Read's growth is bounded, its cap is exact at N and N+1, and
# a refusal is recorded as payload_too_large. Reading through io.ReadAll's
# doubling, moving the cap by one byte, or folding the refusal into
# internal_error is each the whole regression.
m("RES-4 the payload is read with io.ReadAll's doubling", "internal/hookio/read.go",
  "\tvar chunks [][]byte\n",
  "\tif b, err := io.ReadAll(io.LimitReader(r, MaxPayloadBytes+1)); err != nil || len(b) <= MaxPayloadBytes {\n\t\treturn b, err\n\t}\n\treturn nil, ErrTooLarge\n\tvar chunks [][]byte\n",
  "TestRES4_")
m("RES-4 a payload of exactly the cap is refused", "internal/hookio/read.go",
  "\t\tif total > MaxPayloadBytes {\n", "\t\tif total >= MaxPayloadBytes {\n", "TestRES4_")
m("RES-4 the refusal is recorded as an internal error", "internal/hook/payload.go",
  "\tcase errors.Is(err, hookio.ErrTooLarge):\n\t\treturn store.ReasonPayloadTooLarge\n",
  "\tcase errors.Is(err, hookio.ErrTooLarge):\n\t\treturn store.ReasonInternalError\n", "TestRES4_")
