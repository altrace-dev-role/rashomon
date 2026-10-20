m("H-2  terminal record never written", "internal/hook/handle.go",
  "\tif h.opened || (h.toolUseID != \"\" && reason == store.ReasonLockTimeout) {", "\tif false {", "TestH2_HealthyPath")
m("H-2  transient settings read is not retried", "internal/hook/coverage.go",
  "\tdoc, err := settings.Load(path)\n\tfor attempt := 1; err != nil && attempt < settingsAttempts; attempt++ {\n\t\ttime.Sleep(settingsRetryPause)\n\t\tdoc, err = settings.Load(path)\n\t}\n\treturn doc, err",
  "\treturn settings.Load(path)", "TestH2_TransientSettingsReadIsRetried")
m("H-2  settings read is retried without a bound", "internal/hook/coverage.go",
  "\tsettingsAttempts   = 3\n", "\tsettingsAttempts   = 20\n", "TestH2_PersistentSettingsReadIsUnresolved")
