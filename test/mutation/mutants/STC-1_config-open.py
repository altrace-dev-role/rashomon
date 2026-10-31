# STC-1 (card STEER-CLI1, claims no requirement row), census #160: a config.json
# that is there and cannot be opened is unreadable, as one that does not parse
# is, so the hook's clamp and `steer on --repair` agree. The break returns the
# open's raw error, which the repair does not recognise.
m("STC-1 an unopenable config is a raw error, not unreadable", "internal/store/config.go",
  "\tif err != nil {\n\t\treturn Config{}, unreadableConfig{err}\n\t}\n\tdefer f.Close()",
  "\tif err != nil {\n\t\treturn Config{}, err\n\t}\n\tdefer f.Close()", "TestSTC1_UnopenableConfigIsUnreadable|TestSTC1o_RepairReplacesAConfigThatCannotBeOpened")
