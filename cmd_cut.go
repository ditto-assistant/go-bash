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
						_, err := fmt.Fprintln(e.Stdout, line)
						return err
					}
					return nil
				}
				parts := strings.Split(line, delimiter)
				indexes := expandSelection(selected, len(parts))
				out := make([]string, 0, len(indexes))
				for _, index := range indexes {
					out = append(out, parts[index-1])
				}
				_, err := fmt.Fprintln(e.Stdout, strings.Join(out, delimiter))
				return err
			}
			runes := []rune(line)
			var out strings.Builder
			for _, index := range expandSelection(selected, len(runes)) {
				out.WriteRune(runes[index-1])
			}
			_, err := fmt.Fprintln(e.Stdout, out.String())
			return err
		})
	})
}

func parseCutArgs(e *Env) (mode rune, spec, delimiter string, onlyDelimited bool, operands []string, ok bool) {
	delimiter = "\t"
	args := e.Args[1:]
	endOfFlags := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if endOfFlags {
			operands = append(operands, a)
			continue
		}
		switch {
		case a == "--":
			endOfFlags = true
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
		case strings.HasPrefix(a, "-"):
			e.Errorf("unsupported option: %s", a)
			return 0, "", "", false, nil, false
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

type selectionRange struct {
	start int
	end   int // zero means through the final field/character
}

func parseSelection(spec string) ([]selectionRange, error) {
	var out []selectionRange
	for _, part := range strings.Split(spec, ",") {
		bounds := strings.SplitN(part, "-", 2)
		start, err := strconv.Atoi(bounds[0])
		if err != nil || start < 1 {
			return nil, fmt.Errorf("%s", part)
		}
		end := start
		if len(bounds) == 2 {
			if bounds[1] == "" {
				end = 0
			} else {
				end, err = strconv.Atoi(bounds[1])
				if err != nil || end < start {
					return nil, fmt.Errorf("%s", part)
				}
			}
		}
		out = append(out, selectionRange{start: start, end: end})
	}
	return out, nil
}

func expandSelection(ranges []selectionRange, limit int) []int {
	seen := make(map[int]bool)
	var out []int
	for _, selection := range ranges {
		end := selection.end
		if end == 0 || end > limit {
			end = limit
		}
		for index := selection.start; index <= end; index++ {
			if !seen[index] {
				seen[index] = true
				out = append(out, index)
			}
		}
	}
	return out
}
