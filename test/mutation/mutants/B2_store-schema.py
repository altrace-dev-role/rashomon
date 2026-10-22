# B2 -- the version gate itself. Putting a schema-3 field in the TOP-LEVEL
# required is the tempting edit and it invalidates every record already on
# disk, so it gets its own mutation.
m("B2 a schema-3 field is required at every version", "docs/store-schema.json",
  '        "tool_name",\n        "shape",\n', '        "tool_name",\n        "shape",\n        "host_source",\n',
  "TestSchema3|TestStoreSchema")
# Re-anchored when AcceptsKind replaced Accepts: the call kinds' range is
# 1 to CallSchemaVersion; the mutation still drops 3 alone. The schema-4 bump
# added its own twin, "MS the reader stops accepting schema 4", as the
# schema-3 bump added this, and group commit's schema-5 bump adds a third,
# "REC-34 the reader stops accepting schema 5" (REC-34_group-commit.py).
m("B2 the reader stops accepting schema 3", "internal/store/kinds.go",
  "\t\treturn version >= 1 && version <= CallSchemaVersion",
  "\t\treturn version >= 1 && version <= CallSchemaVersion && version != 3", "TestAcceptsAdmits")
