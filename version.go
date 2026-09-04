package gobash

const (
	// Version is the go-bash release version. Keep it aligned with the module tag.
	Version = "v0.4.0"

	// BashCompatibility is the GNU Bash language compatibility target. go-bash
	// deliberately identifies itself with a suffix rather than claiming to be
	// the GNU implementation.
	BashCompatibility = "5.3"
	// BashVersion is the value exposed through the BASH_VERSION environment
	// variable and runtime metadata.
	BashVersion = "5.3.15(1)-go-bash"

	// Runtime identifies the interpreter implementation independently of the
	// Bash language compatibility target.
	Runtime = "go-bash/mvdan-sh"
	// RuntimeVersion identifies the embedded mvdan/sh interpreter version.
	RuntimeVersion = "mvdan-sh/v3.13.1"
)
