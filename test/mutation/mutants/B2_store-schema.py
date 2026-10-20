# B2 -- the version gate itself. Putting a schema-3 field in the TOP-LEVEL
# required is the tempting edit and it invalidates every record already on
# disk, so it gets its own mutation.
m("B2 a schema-3 field is required at every version", "docs/store-schema.json",
  '        "tool_name",\n        "shape",\n', '        "tool_name",\n        "shape",\n        "host_source",\n',
  "TestSchema3|TestStoreSchema")
# Re-anchored after the masked-status refactor: Accepts is a range since schema
# 4; the mutation still drops 3 alone. The schema-4 bump adds its own twin,
# "MS the reader stops accepting schema 4", as the schema-3 bump added this.
m("B2 the reader stops accepting schema 3", "internal/store/record.go",
  "\treturn version >= 1 && version <= 4",
  "\treturn version >= 1 && version <= 4 && version != 3", "TestAcceptsAdmits")
