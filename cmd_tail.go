package gobash

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func init() { Register("tail", cmdTail) }

func cmdTail(ctx context.Context, e *Env) int {
	count, operands, ok := parseLineCount(e, 10)
	if !ok {
		return 1
	}
	fromStart := tailFromStart(e.Args[1:])
	multiple := len(operands) > 1
	first := true
	return forEachInput(ctx, e, operands, func(ctx context.Context, name string, r io.Reader) error {
		if fromStart {
			return scanLines(ctx, r, func(line string, lineNo int) error {
				if lineNo >= count {
					_, err := fmt.Fprintln(e.Stdout, line)
					return err
				}
				return nil
			})
		}
		lines := make([]string, 0, count)
		err := scanLines(ctx, r, func(line string, _ int) error {
			if count == 0 {
				return nil
			}
			if len(lines) < count {
				lines = append(lines, line)
				return nil
			}
			copy(lines, lines[1:])
			lines[len(lines)-1] = line
			return nil
		})
		if err != nil {
			return err
		}
		if multiple {
			if !first {
				if _, err := fmt.Fprintln(e.Stdout); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(e.Stdout, "==> %s <==\n", name); err != nil {
				return err
			}
			first = false
		}
		for _, line := range lines {
			if _, err := fmt.Fprintln(e.Stdout, line); err != nil {
				return err
			}
		}
		return nil
	})
}

func tailFromStart(args []string) bool {
	for i, arg := range args {
		switch {
		case arg == "-n" || arg == "--lines":
			return i+1 < len(args) && strings.HasPrefix(args[i+1], "+")
		case strings.HasPrefix(arg, "-n+") || strings.HasPrefix(arg, "--lines=+"):
			return true
		}
	}
	return false
}
