module github.com/bazelbuild/bazelisk

go 1.24.0

toolchain go1.24.2

require (
	github.com/bgentry/go-netrc v0.0.0-20140422174119-9fd32a8b3d3d
	github.com/gofrs/flock v0.12.1
	github.com/hashicorp/go-version v1.7.0
	github.com/mitchellh/go-homedir v1.1.0
	golang.org/x/term v0.35.0
)

require golang.org/x/sys v0.36.0 // indirect
