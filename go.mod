module github.com/altrace-dev-role/rashomon

// No `toolchain` directive on purpose, matching the convention in the sibling
// repository: builder images set GOTOOLCHAIN=local, which ignores it, so it
// would declare a version nothing in the release path ever uses.
go 1.24
