// Package gobash provides a simulated bash environment with an in-memory
// filesystem, designed for AI agents. It is a Go port of the ideas behind
// just-bash (https://github.com/vercel-labs/just-bash).
//
// The shell language is interpreted by mvdan.cc/sh/v3 (no external shell
// binary is ever spawned). Filesystem access is backed by an in-memory
// [github.com/spf13/afero] filesystem by default, so scripts cannot touch the
// host disk. The standard Unix commands agents rely on (cat, ls, grep, sed,
// ...) are reimplemented in Go as builtins that operate on the virtual
// filesystem; there is no fall-through to host binaries.
//
// Architecture:
//
//	script ──▶ mvdan/sh parser ──▶ mvdan/sh interp
//	                                   │
//	                 ExecHandlers ─────┤ dispatch argv[0] to a registered
//	                                   │   Go builtin (see Register)
//	          Open/Stat/ReadDir/Access─┘ redirects, cwd, globs and tests hit
//	                                     the in-memory afero filesystem
package gobash
