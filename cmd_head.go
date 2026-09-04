package gobash

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func init() { Register("head", cmdHead) }

func cmdHead(ctx context.Context, e *Env) int {
	count, operands, ok := parseLineCount(e, 10)
	if !ok {
		return 1
	}
	multiple := len(operands) > 1
	first := true
	return forEachInput(ctx, e, operands, func(ctx context.Context, name string, r io.Reader) error {
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
		return scanLines(ctx, r, func(line string, lineNo int) error {
			if lineNo <= count {
				_, err := fmt.Fprintln(e.Stdout, line)
				return err
			}
			return nil
		})
	})
}

func parseLineCount(e *Env, defaultCount int) (int, []string, bool) {
	count := defaultCount
	args := e.Args[1:]
	operands := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			operands = append(operands, args[i+1:]...)
			return count, operands, true
		case a == "-n" || a == "--lines":
			if i+1 >= len(args) {
				e.Errorf("option requires an argument -- 'n'")
				return 0, nil, false
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 {
				e.Errorf("invalid number of lines: %s", args[i])
				return 0, nil, false
			}
			count = n
		case strings.HasPrefix(a, "-n") && len(a) > 2:
			n, err := strconv.Atoi(strings.TrimPrefix(a, "-n"))
			if err != nil || n < 0 {
				e.Errorf("invalid number of lines: %s", a)
				return 0, nil, false
			}
			count = n
		case strings.HasPrefix(a, "--lines="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--lines="))
			if err != nil || n < 0 {
				e.Errorf("invalid number of lines: %s", a)
				return 0, nil, false
			}
			count = n
		case len(a) > 1 && a[0] == '-' && allDigits(a[1:]):
			n, _ := strconv.Atoi(a[1:])
			count = n
		case strings.HasPrefix(a, "-") && a != "-":
			e.Errorf("unsupported option: %s", a)
			return 0, nil, false
		default:
			operands = append(operands, a)
		}
	}
	return count, operands, true
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
