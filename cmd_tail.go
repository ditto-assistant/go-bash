package gobash

import (
	"context"
	"fmt"
	"io"
)

func init() { Register("tail", cmdTail) }

func cmdTail(ctx context.Context, e *Env) int {
	count, operands, ok := parseLineCount(e, 10)
	if !ok {
		return 1
	}
	multiple := len(operands) > 1
	first := true
	return forEachInput(ctx, e, operands, func(ctx context.Context, name string, r io.Reader) error {
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
