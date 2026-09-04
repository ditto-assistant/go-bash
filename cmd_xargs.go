package gobash

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/shell"
)

func init() { Register("xargs", cmdXargs) }

const maxXargsInput = 16 << 20

type xargsOptions struct {
	null    bool
	noRun   bool
	maxArgs int
	replace string
	command []string
}

func cmdXargs(ctx context.Context, e *Env) int {
	if len(e.Args) == 2 && e.Args[1] == "--help" {
		_, _ = fmt.Fprintln(e.Stdout, "usage: xargs [-0r] [-n max-args] [-I replace] [command [initial-args...]]")
		return 0
	}
	opts, ok := parseXargsOptions(e)
	if !ok {
		return 2
	}
	input, err := io.ReadAll(io.LimitReader(e.Stdin, maxXargsInput+1))
	if err != nil {
		e.Errorf("%v", err)
		return 1
	}
	if len(input) > maxXargsInput {
		e.Errorf("input exceeds %d-byte limit", maxXargsInput)
		return 1
	}
	items, err := xargsFields(string(input), opts.null, e.Environ)
	if err != nil {
		e.Errorf("%v", err)
		return 1
	}
	if len(items) == 0 && opts.noRun {
		return 0
	}
	if opts.replace != "" {
		for _, item := range items {
			args := append([]string(nil), opts.command...)
			for i := range args {
				args[i] = strings.ReplaceAll(args[i], opts.replace, item)
			}
			if code := runXargsCommand(ctx, e, args); code != 0 {
				return code
			}
		}
		return 0
	}
	batchSize := opts.maxArgs
	if batchSize <= 0 || batchSize > len(items) {
		batchSize = len(items)
	}
	if len(items) == 0 {
		return runXargsCommand(ctx, e, opts.command)
	}
	for start := 0; start < len(items); start += batchSize {
		end := min(start+batchSize, len(items))
		args := append(append([]string(nil), opts.command...), items[start:end]...)
		if code := runXargsCommand(ctx, e, args); code != 0 {
			return code
		}
	}
	return 0
}

func parseXargsOptions(e *Env) (xargsOptions, bool) {
	opts := xargsOptions{command: []string{"echo"}}
	for i := 1; i < len(e.Args); i++ {
		arg := e.Args[i]
		switch {
		case arg == "-0" || arg == "--null":
			opts.null = true
		case arg == "-r" || arg == "--no-run-if-empty":
			opts.noRun = true
		case arg == "-n" || arg == "--max-args":
			if i+1 >= len(e.Args) {
				e.Errorf("option %s requires an argument", arg)
				return opts, false
			}
			i++
			value, err := strconv.Atoi(e.Args[i])
			if err != nil || value <= 0 {
				e.Errorf("invalid max-args %q", e.Args[i])
				return opts, false
			}
			opts.maxArgs = value
		case strings.HasPrefix(arg, "-n") && len(arg) > 2:
			value, err := strconv.Atoi(arg[2:])
			if err != nil || value <= 0 {
				e.Errorf("invalid max-args %q", arg[2:])
				return opts, false
			}
			opts.maxArgs = value
		case arg == "-I" || arg == "--replace":
			if i+1 >= len(e.Args) {
				e.Errorf("option %s requires an argument", arg)
				return opts, false
			}
			i++
			opts.replace = e.Args[i]
		case strings.HasPrefix(arg, "-I") && len(arg) > 2:
			opts.replace = arg[2:]
		default:
			opts.command = append([]string(nil), e.Args[i:]...)
			return opts, true
		}
	}
	return opts, true
}

func xargsFields(input string, nullSeparated bool, environ []string) ([]string, error) {
	if nullSeparated {
		input = strings.TrimSuffix(input, "\x00")
		if input == "" {
			return nil, nil
		}
		return strings.Split(input, "\x00"), nil
	}
	values := make(map[string]string, len(environ))
	for _, pair := range environ {
		name, value, _ := strings.Cut(pair, "=")
		values[name] = value
	}
	return shell.Fields(input, func(name string) string { return values[name] })
}

func runXargsCommand(ctx context.Context, e *Env, args []string) int {
	if len(args) == 0 || args[0] == "echo" {
		if len(args) > 1 {
			if _, err := fmt.Fprintln(e.Stdout, strings.Join(args[1:], " ")); err != nil {
				e.Errorf("%v", err)
				return 1
			}
			return 0
		}
		_, _ = fmt.Fprintln(e.Stdout)
		return 0
	}
	if e.RunCommand == nil {
		e.Errorf("command dispatcher unavailable")
		return 127
	}
	return e.RunCommand(ctx, args)
}
