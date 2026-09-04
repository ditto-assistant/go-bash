package gobash

import (
	"context"
	"fmt"
	"strings"
)

func init() { Register("env", cmdEnv) }

func cmdEnv(_ context.Context, e *Env) int {
	separator := "\n"
	for _, arg := range e.Args[1:] {
		switch arg {
		case "-0", "--null":
			separator = "\x00"
		case "--help":
			_, _ = fmt.Fprintln(e.Stdout, "usage: env [-0|--null]")
			return 0
		default:
			e.Errorf("assignments and command execution are unavailable; use shell export and invoke the command directly")
			return 2
		}
	}
	if len(e.Environ) == 0 {
		return 0
	}
	if _, err := fmt.Fprint(e.Stdout, strings.Join(e.Environ, separator)+separator); err != nil {
		e.Errorf("%v", err)
		return 1
	}
	return 0
}
