package gobash

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

type runStateKey struct{}

type runState struct {
	commands    atomic.Int32
	maxCommands int
}

// execMiddleware dispatches each simple command to a registered Go builtin.
// There is deliberately no fall-through to the host operating system: an
// unknown command behaves like bash's "command not found" (exit code 127).
// This is what keeps the environment safe without an OS-level sandbox.
func (s *Shell) execMiddleware(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		if len(args) == 0 {
			return next(ctx, args)
		}
		hc := interp.HandlerCtx(ctx)
		code := s.runCommand(ctx, args, hc)
		if code == 0 {
			return nil
		}
		return interp.ExitStatus(uint8(code))
	}
}

func (s *Shell) runCommand(ctx context.Context, args []string, hc interp.HandlerContext) int {
	if state, _ := ctx.Value(runStateKey{}).(*runState); state != nil &&
		state.commands.Add(1) > int32(state.maxCommands) {
		_, _ = fmt.Fprintf(hc.Stderr, "gobash: command limit (%d) exceeded\n", state.maxCommands)
		return 124
	}
	if err := ctx.Err(); err != nil {
		return 124
	}
	fn, ok := lookup(args[0])
	if !ok {
		_, _ = fmt.Fprintf(hc.Stderr, "%s: command not found\n", args[0])
		return 127
	}
	runNested := func(nestedCtx context.Context, nestedArgs []string) int {
		return s.runShellCommand(nestedCtx, nestedArgs, hc)
	}
	env := &Env{
		Args: args, Stdin: hc.Stdin, Stdout: hc.Stdout, Stderr: hc.Stderr,
		FS: s.fs, Dir: hc.Dir, Environ: exportedEnvironment(hc.Env),
		Now: s.now, RunCommand: runNested,
	}
	return fn(ctx, env)
}

// runShellCommand safely re-enters the interpreter with one argv vector. The
// quoting prevents xargs input from becoming shell syntax, while using an
// interpreter runner lets commands such as printf resolve as shell builtins.
func (s *Shell) runShellCommand(ctx context.Context, args []string, hc interp.HandlerContext) int {
	if len(args) == 0 {
		return 0
	}
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(commandPrelude()+strings.Join(quoted, " ")), "xargs")
	if err != nil {
		_, _ = fmt.Fprintf(hc.Stderr, "xargs: parse command: %v\n", err)
		return 2
	}
	runner, err := interp.New(
		interp.StdIO(hc.Stdin, hc.Stdout, hc.Stderr),
		interp.Dir(hc.Dir),
		interp.Env(hc.Env),
		interp.ExecHandlers(s.execMiddleware),
		interp.OpenHandler(s.openHandler),
		interp.StatHandler(s.statHandler),
		interp.ReadDirHandler2(s.readDirHandler),
	)
	if err != nil {
		_, _ = fmt.Fprintf(hc.Stderr, "xargs: initialize command: %v\n", err)
		return 2
	}
	if err := runner.Run(ctx, file); err != nil {
		var status interp.ExitStatus
		if errors.As(err, &status) {
			return int(status)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return 124
		}
		if errors.Is(err, context.Canceled) {
			return 130
		}
		_, _ = fmt.Fprintf(hc.Stderr, "xargs: run command: %v\n", err)
		return 1
	}
	return 0
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func exportedEnvironment(environ expand.Environ) []string {
	values := make(map[string]string)
	environ.Each(func(name string, variable expand.Variable) bool {
		if variable.IsSet() && variable.Exported && variable.Kind == expand.String {
			values[name] = variable.String()
		}
		return true
	})
	pairs := make([]string, 0, len(values))
	for name, value := range values {
		pairs = append(pairs, name+"="+value)
	}
	sort.Strings(pairs)
	return pairs
}

// commandPrelude makes every registered command visible to mvdan's `command
// -v` builtin. Each function immediately re-enters the fail-closed dispatcher;
// no host executable is resolved or invoked.
func commandPrelude() string {
	var b strings.Builder
	for _, name := range Commands() {
		fmt.Fprintf(&b, "%s() { command %s \"$@\"; }\n", name, name)
	}
	return b.String()
}
