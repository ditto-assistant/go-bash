package gobash

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func init() { Register("uniq", cmdUniq) }

func cmdUniq(ctx context.Context, e *Env) int {
	flags, operands := splitArgs(e.Args[1:])
	if !validateShortFlags(e, flags, "cdui") {
		return 2
	}
	if len(operands) > 1 {
		e.Errorf("extra operand %s", operands[1])
		return 1
	}
	return forEachInput(ctx, e, operands, func(ctx context.Context, _ string, r io.Reader) error {
		var previous, key string
		count := 0
		flush := func() error {
			if count == 0 || (flags['d'] && count == 1) || (flags['u'] && count != 1) {
				return nil
			}
			if flags['c'] {
				_, err := fmt.Fprintf(e.Stdout, "%7d %s\n", count, previous)
				return err
			}
			_, err := fmt.Fprintln(e.Stdout, previous)
			return err
		}
		err := scanLines(ctx, r, func(line string, _ int) error {
			lineKey := line
			if flags['i'] {
				lineKey = strings.ToLower(line)
			}
			if count > 0 && lineKey != key {
				if err := flush(); err != nil {
					return err
				}
				count = 0
			}
			previous, key, count = line, lineKey, count+1
			return nil
		})
		if err != nil {
			return err
		}
		return flush()
	})
}
