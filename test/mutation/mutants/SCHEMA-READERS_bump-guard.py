# SCHEMA-READERS: a call-schema bump must teach every reader the new version
# (D11). The guard test refuses a reader that closes the range with a literal
# and an acceptance rule that stops short of CallSchemaVersion.
m("SCHEMA-READERS the store stops accepting the current call version", "internal/store/kinds.go",
  "return version >= 1 && version <= CallSchemaVersion",
  "return version >= 1 && version < CallSchemaVersion",
  "TestSchemaReaders_")
m("SCHEMA-READERS a reader pins the bit's version with an equality", "internal/report/account.go",
  "if d.SchemaVersion >= 9 && d.LocationUnread {",
  "if d.SchemaVersion == 9 && d.LocationUnread {",
  "TestSchemaReaders_")
