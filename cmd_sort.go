package gobash

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
)

func init() { Register("sort", cmdSort) }

func cmdSort(ctx context.Context, e *Env) int {
	flags, operands := splitArgs(e.Args[1:])
	var lines []string
	code := forEachInput(ctx, e, operands, func(ctx context.Context, _ string, r io.Reader) error {
		return scanLines(ctx, r, func(line string, _ int) error {
			lines = append(lines, line)
			return nil
		})
	})
	if code != 0 {
		return code
	}
	less := func(i, j int) bool { return lines[i] < lines[j] }
	if flags['n'] {
		less = func(i, j int) bool {
			a, _ := strconv.ParseFloat(lines[i], 64)
			b, _ := strconv.ParseFloat(lines[j], 64)
			return a < b
		}
	}
	sort.SliceStable(lines, less)
	if flags['r'] {
		for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
			lines[i], lines[j] = lines[j], lines[i]
		}
	}
	last := ""
	for i, line := range lines {
		if flags['u'] && i > 0 && line == last {
			continue
		}
		if _, err := fmt.Fprintln(e.Stdout, line); err != nil {
			e.Errorf("%v", err)
			return 1
		}
		last = line
	}
	return 0
}
