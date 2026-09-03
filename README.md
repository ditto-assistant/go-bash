# go-bash

A simulated bash environment with an in-memory filesystem, for AI agents — a Go
port of the ideas behind [just-bash](https://github.com/vercel-labs/just-bash).

Agents need a shell, but giving them a *real* shell is risky. `go-bash` gives
them a real bash *language* with pipes, redirects, variables, functions, loops
and globs — but the filesystem is virtual (in-memory), network is opt-in, and
nothing touches the host. No OS-level sandbox required.

```go
sh := gobash.New() // fresh in-memory filesystem rooted at "/"
res, _ := sh.Run(context.Background(), `
  mkdir -p /data
  echo "hello" > /data/greeting.txt
  cat /data/greeting.txt | tr a-z A-Z
`)
fmt.Print(res.Stdout) // HELLO
```

## Architecture

`go-bash` is deliberately a thin assembly over two mature libraries plus a layer
of reimplemented coreutils:

| Layer | Provided by | Notes |
|-------|-------------|-------|
| Shell **language** (parse + interpret) | [`mvdan.cc/sh/v3`](https://github.com/mvdan/sh) | Pure Go, no external shell. Pipes, redirects, `&&`/`||`, loops, functions, globs, heredocs, expansion. |
| Shell **builtins** (`echo`, `printf`, `pwd`, `cd`, `test`, `[`, `true`, `false`, `read`, `getopts`, ...) | `mvdan.cc/sh/v3/interp` | Handled internally by the interpreter — **not** reimplemented here. |
| **Virtual filesystem** | [`spf13/afero`](https://github.com/spf13/afero) `MemMapFs` | In-memory, concurrent-safe, read/write. |
| **External coreutils** (`cat`, `ls`, `mkdir`, `rm`, `grep`, `sed`, `awk`, `jq`, ...) | this repo | Go builtins dispatched via the interpreter's `ExecHandlers`, operating on the afero FS. **This is where the work is.** |

The interpreter's `ExecHandlers` middleware dispatches `argv[0]` to a registered
Go builtin; there is **no fall-through to host binaries** — an unknown command
is a `command not found` (exit 127). The `Open`/`Stat`/`ReadDir` handlers route
redirects, globbing and path tests to the same in-memory filesystem.

See [`AGENTS.md`](./AGENTS.md) for the contributor contract (how to add a
command, the test/TDD model, and the boundary rules).

## Included command set

The initial agent/data-analysis toolset is implemented end-to-end:
`cat`, `ls`, `mkdir`, `touch`, `rm`, `cp`, `mv`, `tree`, `head`, `tail`, `wc`,
`sort`, `uniq`, `cut`, `tr`, `grep`/`egrep`/`fgrep`, `rg`, `sed`, `awk`, `find`,
`basename`, `dirname`, `jq`, `base64`, and `tee`.

`Shell.Run` also bounds script bytes, captured output, and external command
invocations. Callers should still provide a deadline through `context.Context`.

## License

TBD (pending decision — see repo owner).
