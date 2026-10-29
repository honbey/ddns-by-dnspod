package config

// Version is the semantic version of the binary, stamped at link time from
// info.version in backend/openapi.yml. A plain `go build` leaves the default,
// which is what an unstamped binary should say.
var Version = "dev"
