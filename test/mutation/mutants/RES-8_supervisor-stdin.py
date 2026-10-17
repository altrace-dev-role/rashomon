# RES-8: the supervisor streams the payload through one buffer and gives the
# child at most the intake cap plus one byte, so a payload over the cap is
# still the child's to refuse as payload_too_large, read to its end.
m("RES-8 the child is given the whole payload, past the cap", "internal/supervise/run_unix.go",
  "\t\t\t\tk := min(int64(n), capBytes-sent)\n", "\t\t\t\tk := int64(n)\n", "TestRES8b_")
m("RES-8 the cap is the intake limit itself, so one byte over reads as whole", "cmd/rashomon/supervised.go",
  "\t\tStdinCap:     hookio.MaxPayloadBytes + 1,\n", "\t\tStdinCap:     hookio.MaxPayloadBytes,\n", "TestRES8_")
