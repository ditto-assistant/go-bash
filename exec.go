package gobash

import (
	"context"
	"fmt"

	"mvdan.cc/sh/v3/interp"
)

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
		fn, ok := lookup(args[0])
		if !ok {
			fmt.Fprintf(hc.Stderr, "%s: command not found\n", args[0])
			return interp.ExitStatus(127)
		}
		env := &Env{
			Args:   args,
			Stdin:  hc.Stdin,
			Stdout: hc.Stdout,
			Stderr: hc.Stderr,
			FS:     s.fs,
			Dir:    hc.Dir,
		}
		code := fn(ctx, env)
		if code == 0 {
			return nil
		}
		return interp.ExitStatus(uint8(code))
	}
}
