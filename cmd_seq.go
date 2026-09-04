package gobash

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func init() { Register("seq", cmdSeq) }

const maxSeqValues = 100_000

func cmdSeq(ctx context.Context, e *Env) int {
	separator := "\n"
	equalWidth := false
	var operands []string
	for i := 1; i < len(e.Args); i++ {
		switch {
		case e.Args[i] == "-w" || e.Args[i] == "--equal-width":
			equalWidth = true
		case e.Args[i] == "-s" || e.Args[i] == "--separator":
			if i+1 >= len(e.Args) {
				e.Errorf("option %s requires an argument", e.Args[i])
				return 2
			}
			i++
			separator = e.Args[i]
		case strings.HasPrefix(e.Args[i], "-s") && len(e.Args[i]) > 2:
			separator = e.Args[i][2:]
		case strings.HasPrefix(e.Args[i], "--separator="):
			separator = strings.TrimPrefix(e.Args[i], "--separator=")
		case e.Args[i] == "--help":
			_, _ = fmt.Fprintln(e.Stdout, "usage: seq [-w] [-s separator] [first [increment]] last")
			return 0
		case strings.HasPrefix(e.Args[i], "-") && len(e.Args[i]) > 1 && !allNumeric(e.Args[i]):
			e.Errorf("unsupported option: %s", e.Args[i])
			return 2
		default:
			operands = append(operands, e.Args[i])
		}
	}
	if len(operands) < 1 || len(operands) > 3 {
		e.Errorf("expected [first [increment]] last")
		return 2
	}
	values := make([]int64, len(operands))
	for i, operand := range operands {
		value, err := strconv.ParseInt(operand, 10, 64)
		if err != nil {
			e.Errorf("invalid integer %q", operand)
			return 2
		}
		values[i] = value
	}
	first, increment, last := int64(1), int64(1), values[0]
	if len(values) == 2 {
		first, last = values[0], values[1]
	}
	if len(values) == 3 {
		first, increment, last = values[0], values[1], values[2]
	}
	if increment == 0 {
		e.Errorf("zero increment")
		return 2
	}
	width := 0
	if equalWidth {
		width = max(len(strconv.FormatInt(first, 10)), len(strconv.FormatInt(last, 10)))
	}
	written := 0
	for value := first; increment > 0 && value <= last || increment < 0 && value >= last; value += increment {
		if err := ctx.Err(); err != nil {
			return 124
		}
		if written >= maxSeqValues {
			e.Errorf("output exceeds %d-value limit", maxSeqValues)
			return 1
		}
		if written > 0 {
			if _, err := fmt.Fprint(e.Stdout, separator); err != nil {
				e.Errorf("%v", err)
				return 1
			}
		}
		formatted := strconv.FormatInt(value, 10)
		if equalWidth {
			formatted = strings.Repeat("0", width-len(formatted)) + formatted
		}
		if _, err := fmt.Fprint(e.Stdout, formatted); err != nil {
			e.Errorf("%v", err)
			return 1
		}
		written++
	}
	if written > 0 {
		_, _ = fmt.Fprintln(e.Stdout)
	}
	return 0
}

func allNumeric(value string) bool {
	_, err := strconv.ParseInt(value, 10, 64)
	return err == nil
}
