# TRU-57: the report sections carry exactly the allowlisted keys. A field that
# comes back is a contract change, and the first one that would is the one
# the proxy era left behind.
m("TRU-57 destinations carries observed again", "internal/report/destinations.go",
  '\tDeclaredHosts []string `json:"declared_hosts"`\n',
  '\tObserved bool `json:"observed"`\n\tDeclaredHosts []string `json:"declared_hosts"`\n', "TestTRU57_")
# What the tag walk cannot see, and the marshalled output can: a key dropped
# by omitzero, a key a tag of "-," emits, a key promoted from an embedded
# struct, and a key a MarshalJSON method writes. Each passed the tag walk in
# review, with the real output changed.
m("TRU-57 novelty drops known_hosts when it is zero", "internal/report/destinations.go",
  '\tKnownHosts int `json:"known_hosts"`\n', '\tKnownHosts int `json:"known_hosts,omitzero"`\n', "TestTRU57_")
m("TRU-57 a family drops hosts_declared when it is zero", "internal/report/families.go",
  '\tHostsDeclared int `json:"hosts_declared"`\n', '\tHostsDeclared int `json:"hosts_declared,omitzero"`\n',
  "TestTRU57_")
m("TRU-57 a link carries a key a tag of \"-,\" names", "internal/report/chains.go",
  '\tSSHHosts []string `json:"ssh_hosts"`\n}\n',
  '\tSSHHosts []string `json:"ssh_hosts"`\n\tState    string   `json:"-,"`\n}\n', "TestTRU57_")
m("TRU-57 a link carries a key promoted from an embedded struct", "internal/report/chains.go",
  '\tSSHHosts []string `json:"ssh_hosts"`\n}\n',
  '\tSSHHosts []string `json:"ssh_hosts"`\n\tlinkWire\n}\n\ntype linkWire struct {\n\tState string `json:"state"`\n}\n',
  "TestTRU57_")
m("TRU-57 a link writes a key of its own in MarshalJSON", "internal/report/chains.go",
  '\tSSHHosts []string `json:"ssh_hosts"`\n}\n',
  '\tSSHHosts []string `json:"ssh_hosts"`\n}\n\nfunc (l Link) MarshalJSON() ([]byte, error) {\n'
  '\ttype plain Link\n\treturn json.Marshal(struct {\n\t\tplain\n\t\tState string `json:"state"`\n'
  '\t}{plain(l), "reached"})\n}\n',
  "TestTRU57_")
IMPORTS["TRU-57 a link writes a key of its own in MarshalJSON"] = (
  "internal/report/chains.go", '\t"sort"\n', '\t"encoding/json"\n\t"sort"\n')
# The omittable set is the specification's, exactly: nono.reason,
# novelty.reason and a link's program. A key that stopped being omitted is a
# contract change too, so the set is pinned in both directions.
m("TRU-57 nono.reason is rendered when empty", "internal/report/nono.go",
  '\tReason   string `json:"reason,omitempty"`\n', '\tReason   string `json:"reason"`\n', "TestTRU57_")
