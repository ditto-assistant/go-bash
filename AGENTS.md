# go-bash — contributor & agent contract

This file is the architecture spec **and** the contract for anyone (human or
agent) adding commands. Read it fully before writing code.

## What this is

A Go port of just-bash: a virtual bash environment for AI agents. Shell language
= `mvdan.cc/sh/v3`. Filesystem = in-memory `spf13/afero`. The coreutils that
agents call (`cat`, `ls`, `grep`, `sed`, `awk`, `jq`, ...) are reimplemented in
Go as builtins operating on the virtual filesystem.

## The boundary — do NOT reimplement these

The interpreter handles these as **internal shell builtins**; they never reach
our dispatcher, so registering them is dead code (the registry panics on
duplicates anyway):

```
true false exit set shift unset echo printf break pwd cd wait builtin
type eval source [ test exec command dirs pushd popd return read getopts shopt
```

Everything else an agent would type (`cat ls mkdir mv cp rm ln touch tree head
tail wc sort uniq cut tr grep sed awk find jq base64 ...`) is an **external
command** and IS our job.

Commands that invoke another argv vector, such as `xargs`, must use
`Env.RunCommand`. That callback safely re-enters the sandbox interpreter for one
quoted command, so it resolves both shell builtins and registered pure-Go
commands. It never falls through to host executables and must not be replaced
with `os/exec`.

## How to add a command

One file per command: `cmd_<name>.go`, plus `cmd_<name>_test.go`. Self-register
in `init` so commands stay independent (no shared registry file to merge):

```go
package gobash

import "context"

func init() { Register("wc", cmdWc) }

// cmdWc counts lines, words and bytes. <document supported flags here>
func cmdWc(ctx context.Context, e *Env) int {
    // - read operands from e.Args[1:]; e.Args[0] is the command name
    // - parse short flags with splitArgs (see flags.go)
    // - read stdin from e.Stdin; write results to e.Stdout
    // - access files ONLY via e.FS, resolving paths with e.Resolve(path)
    // - report errors with e.Errorf(...) and return a non-zero exit code
    return 0
}
```

Rules:

- **Filesystem only via `e.FS`** (an `afero.Fs`). Never import `os` for file
  access — that would escape the sandbox. Resolve relative paths with
  `e.Resolve`.
- **Streams**: read `e.Stdin`, write `e.Stdout` / `e.Stderr`. Never write to the
  process's real stdio.
- **Exit codes** mirror coreutils: 0 success, 1 for general errors, 2 for usage
  errors where the real tool uses 2.
- **No panics** in command paths; return error codes. No `os.Exit`.

## TDD model

Generate expected outputs from **real bash + coreutils** and assert `go-bash`
matches. The corpus lives in `testdata/` as cases of
`{script, stdin, want_stdout, want_exit}`. For text-processing commands
(`grep`/`sed`/`tr`/`sort`/...) cases are FS-independent and can be compared
directly to host `bash -c`. For filesystem commands, set up equivalent state in
both a host temp dir and the virtual FS, then compare.

Every command needs table-driven tests covering: no operands (stdin), multiple
operands, missing file, the documented flags, and an empty-input edge case.

## Code style

- Modern Go (1.26+); follow *100 Go Mistakes*. Prefer `io`/`bufio` streaming over
  reading whole files into memory where inputs may be large.
- Wrap errors with `%w`; check every error. Keep functions small.
- `go vet ./...` and `golangci-lint run` must pass. `go test -race ./...` green.

## Command set (build order)

1. **File ops**: `cat ls mkdir touch rm cp mv ln tree pwd*` (`pwd` is a builtin)
2. **Text**: `head tail wc sort uniq cut tr nl rev tac`
3. **Search/transform**: `grep sed find basename dirname`
4. **Heavy** (mini-interpreters, own sub-packages): `awk jq`
5. **Data/encoding**: `base64 xxd od hexdump gzip gunzip tar zip unzip`

Items 1–3 are the priority. `awk`/`jq` are large and may be staged behind a
build tag or a sub-package; they should not block the rest.
