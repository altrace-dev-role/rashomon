# PF-PL9: the go tool the loader runs is offline whatever the caller's
# environment asks. Each row drops one pin from forced(); TestPL9_LoaderEnvForced
# gives most pins a hostile value, wants each one exactly, and counts them.
# GO_TELEMETRY_CHILD's row is judged by behaviour too: the go tool run in
# LoaderEnv takes no telemetry upload token (TestPL9_NoTelemetrySidecar).
# Both tests are the pgraph module's own, which `go test ./...` from the root
# never enters, so the rows run there.
MODULE = "pgraph"
F = "pgraph/ingest.go"
T = "TestPL9_LoaderEnvForced"
for pin in ['"GOPROXY=off"', '"GOTOOLCHAIN=local"', '"GOSUMDB=off"', '"GOFLAGS=-mod=readonly"', '"GOWORK=off"',
            '"GOENV=off"', '"GO111MODULE=on"', '"GOPACKAGESDRIVER=off"', '"CGO_ENABLED=0"',
            '"GOOS=" + c.GOOS', '"GOARCH=" + c.GOARCH']:
    m("PL9 the loader leaves " + pin.split("=")[0].strip('"') + " to the caller", F, pin + ",", "", T)
m("PL9 the loader leaves GO_TELEMETRY_CHILD to the caller", F, '"GO_TELEMETRY_CHILD=2",', "",
  T + "|TestPL9_NoTelemetrySidecar")
