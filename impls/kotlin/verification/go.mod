// Its own module, so `go build ./...` at the repository root does not pull the
// Kotlin corner's verification driver into the rig's module. The Go corner is
// vendored the same way, for the same reason.
module twitterport/kotlin-verification

go 1.25
