# REC-27: the session record carries one claude_code_version field, with its
# source beside it. A second field under the same key is the whole regression:
# encoding/json drops both, and the version silently leaves the record.
m("REC-27 the session record carries claude_code_version twice", "internal/store/record_v5.go",
  "`json:\"claude_code_version_source\"`",
  "`json:\"claude_code_version\"`", "TestREC27b_")
