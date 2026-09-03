package gobash

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func init() { Register("cut", cmdCut) }

func cmdCut(ctx context.Context, e *Env) int {
	mode, spec, delimiter, onlyDelimited, operands, ok := parseCutArgs(e)
	if !ok {
		return 2
	}
	selected, err := parseSelection(spec)
	if err != nil {
		e.Errorf("invalid field/character list: %v", err)
		return 2
	}
	return forEachInput(ctx, e, operands, func(ctx context.Context, _ string, r io.Reader) error {
		return scanLines(ctx, r, func(line string, _ int) error {
			if mode == 'f' {
				if !strings.Contains(line, delimiter) {
					if !onlyDelimited {
						fmt.Fprintln(e.Stdout, line)
					}
					return nil
				}
				parts := strings.Split(line, delimiter)
				out := make([]string, 0, len(selected))
				for _, index := range selected {
					if index > 0 && index <= len(parts) {
						out = append(out, parts[index-1])
					}
				}
				fmt.Fprintln(e.Stdout, strings.Join(out, delimiter))
				return nil
			}
			runes := []rune(line)
			var out strings.Builder
			for _, index := range selected {
				if index > 0 && index <= len(runes) {
					out.WriteRune(runes[index-1])
				}
			}
			fmt.Fprintln(e.Stdout, out.String())
			return nil
		})
	})
}

func parseCutArgs(e *Env) (mode rune, spec, delimiter string, onlyDelimited bool, operands []string, ok bool) {
	delimiter = "\t"
	args := e.Args[1:]
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-s":
			onlyDelimited = true
		case a == "-f" || a == "-c":
			if i+1 >= len(args) {
				e.Errorf("option %s requires an argument", a)
				return 0, "", "", false, nil, false
			}
			i++
			mode, spec = rune(a[1]), args[i]
		case a == "-d":
			if i+1 >= len(args) || len([]rune(args[i+1])) != 1 {
				e.Errorf("delimiter must be one character")
				return 0, "", "", false, nil, false
			}
			i++
			delimiter = args[i]
		case strings.HasPrefix(a, "-f") && len(a) > 2:
			mode, spec = 'f', a[2:]
		case strings.HasPrefix(a, "-c") && len(a) > 2:
			mode, spec = 'c', a[2:]
		case strings.HasPrefix(a, "-d") && len(a) > 2:
			delimiter = a[2:]
		default:
			operands = append(operands, a)
		}
	}
	if mode == 0 || spec == "" {
		e.Errorf("you must specify a list of fields or characters")
		return 0, "", "", false, nil, false
	}
	return mode, spec, delimiter, onlyDelimited, operands, true
}

func parseSelection(spec string) ([]int, error) {
	seen := map[int]bool{}
	var out []int
	for _, part := range strings.Split(spec, ",") {
		bounds := strings.SplitN(part, "-", 2)
		start, err := strconv.Atoi(bounds[0])
		if err != nil || start < 1 {
			return nil, fmt.Errorf("%s", part)
		}
		end := start
		if len(bounds) == 2 {
			end, err = strconv.Atoi(bounds[1])
			if err != nil || end < start {
				return nil, fmt.Errorf("%s", part)
			}
		}
		for i := start; i <= end; i++ {
			if !seen[i] {
				seen[i] = true
				out = append(out, i)
			}
		}
	}
	return out, nil
}
