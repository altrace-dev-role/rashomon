# PW1b-ON-230 step 3: the preflight platform line. decode's keep sees each
# record's object as well as its subject (a symbol's callers are the records
# whose object it is; ARCH-INLINE1 reads the same).
D = "internal/platform/decode.go"

m("PW1b-3 keep sees the subject twice", D,
  "!keep(c[1], c[3])", "!keep(c[1], c[1])", "TestPW1b3_")
