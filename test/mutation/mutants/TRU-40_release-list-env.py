# The release listing shells out to `go list`, and the matrix reader's own
# premise is that it sees what a target really links. A developer's ambient
# GOFLAGS or GOEXPERIMENT must not be able to change that answer.
m("the release listing inherits ambient GOFLAGS and GOEXPERIMENT", "test/acceptance/release_targets_test.go",
  'env := append(os.Environ(), "GOFLAGS=", "GOEXPERIMENT=")',
  'env := append(os.Environ())',
  "TestReleaseListEnv_ClearsAmbientGOFLAGSAndGOEXPERIMENT")
# A build matrix line that genuinely needs to set GOFLAGS or GOEXPERIMENT
# must still win over the forced-empty default; reversing the append order
# would make the default win instead.
m("a build matrix line's own GOFLAGS loses to the forced-empty default", "test/acceptance/release_targets_test.go",
  'env := append(os.Environ(), "GOFLAGS=", "GOEXPERIMENT=")\n\tenv = append(env, extra...)',
  'env := append(os.Environ(), extra...)\n\tenv = append(env, "GOFLAGS=", "GOEXPERIMENT=")',
  "TestReleaseListEnv_ABuildMatrixLineStillWinsOverTheDefault")
