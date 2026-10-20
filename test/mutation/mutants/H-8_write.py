m("H-8  settings written in place instead of temp+rename", "internal/settings/write.go",
  "\ttmp, err := os.CreateTemp(dir, \".settings.json.rashomon-*\")", "\ttmp, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)", "TestH8_")
