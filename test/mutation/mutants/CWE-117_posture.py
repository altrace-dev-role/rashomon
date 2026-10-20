m("CWE-117 the mode refusal repeats connect_mode raw", "internal/posture/posture.go",
  "\"the proxy is in \" + strconv.Quote(v.File.ConnectMode) +", "\"the proxy is in \" + v.File.ConnectMode +",
  "TestRead_AReasonNeverRepeatsAFieldRaw|TestH22_EnvRefusesEveryPostureRunRefuses")
m("CWE-117 the non-loopback refusal repeats listen_addr raw", "internal/posture/posture.go",
  "\t\t\tstrconv.Quote(v.File.ListenAddr) + \"); refusing", "\t\t\tv.File.ListenAddr + \"); refusing",
  "TestRead_AReasonNeverRepeatsAFieldRaw|TestH22_EnvRefusesEveryPostureRunRefuses")
