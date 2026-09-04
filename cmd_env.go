package gobash

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

func init() { Register("env", cmdEnv) }

func cmdEnv(ctx context.Context, e *Env) int {
	separator := "\n"
	assignments := make([]string, 0)
	var command []string
	for i := 1; i < len(e.Args); i++ {
		arg := e.Args[i]
		switch arg {
		case "-0", "--null":
			separator = "\x00"
		case "--":
			command = append(command, e.Args[i+1:]...)
			i = len(e.Args)
		case "--help":
			_, _ = fmt.Fprintln(e.Stdout, "usage: env [-0|--null] [NAME=value ...] [command [args...]]")
			return 0
		default:
			name, _, assignment := strings.Cut(arg, "=")
			if command == nil && assignment && name != "" {
				assignments = append(assignments, arg)
				continue
			}
			command = append(command, e.Args[i:]...)
			i = len(e.Args)
		}
	}
	if len(command) > 0 {
		if e.RunCommandEnv == nil {
			e.Errorf("command dispatcher unavailable")
			return 127
		}
		return e.RunCommandEnv(ctx, command, assignments)
	}
	environ := append([]string(nil), e.Environ...)
	for _, assignment := range assignments {
		name, _, _ := strings.Cut(assignment, "=")
		prefix := name + "="
		for i := len(environ) - 1; i >= 0; i-- {
			if strings.HasPrefix(environ[i], prefix) {
				environ = append(environ[:i], environ[i+1:]...)
			}
		}
		environ = append(environ, assignment)
	}
	slices.Sort(environ)
	if len(environ) == 0 {
		return 0
	}
	if _, err := fmt.Fprint(e.Stdout, strings.Join(environ, separator)+separator); err != nil {
		e.Errorf("%v", err)
		return 1
	}
	return 0
}
