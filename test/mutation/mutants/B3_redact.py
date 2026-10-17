# B3 -- keyed redaction. Both halves: the key itself, and the domain separator
# that keeps this digest from colliding with the one forget --host stores.
m("B3 redaction falls back to an unkeyed hash", "internal/report/redact.go",
  "\tmac := hmac.New(sha256.New, key)", "\tmac := hmac.New(sha256.New, nil)",
  "TestRedact_")
m("B3 redaction reuses the forget-host domain separator", "internal/report/redact.go",
  '\tmac.Write([]byte(redactDomain))', '\tmac.Write([]byte("forget-host\\x00"))',
  "TestRedact_")
