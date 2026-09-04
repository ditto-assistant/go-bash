package gobash

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func init() { Register("tr", cmdTr) }

func cmdTr(ctx context.Context, e *Env) int {
	flags, operands := splitArgs(e.Args[1:])
	if !validateShortFlags(e, flags, "d") {
		return 2
	}
	deleteMode := flags['d']
	if len(operands) < 1 || !deleteMode && len(operands) < 2 {
		e.Errorf("missing operand")
		return 1
	}
	from := expandTrSet(operands[0])
	to := []rune{}
	if !deleteMode {
		to = expandTrSet(operands[1])
	}
	data, err := io.ReadAll(e.Stdin)
	if err != nil {
		e.Errorf("%v", err)
		return 1
	}
	index := make(map[rune]int, len(from))
	for i, r := range from {
		index[r] = i
	}
	var out strings.Builder
	for _, r := range string(data) {
		if err := ctx.Err(); err != nil {
			e.Errorf("%v", err)
			return 1
		}
		i, found := index[r]
		if !found {
			out.WriteRune(r)
			continue
		}
		if deleteMode {
			continue
		}
		if len(to) > 0 {
			if i >= len(to) {
				i = len(to) - 1
			}
			out.WriteRune(to[i])
		}
	}
	_, err = fmt.Fprint(e.Stdout, out.String())
	if err != nil {
		e.Errorf("%v", err)
		return 1
	}
	return 0
}

func expandTrSet(value string) []rune {
	classes := map[string]string{
		"[:alnum:]": "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
		"[:alpha:]": "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ",
		"[:digit:]": "0123456789",
		"[:lower:]": "abcdefghijklmnopqrstuvwxyz",
		"[:space:]": " \t\n\r\f\v",
		"[:upper:]": "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
	}
	if expanded, ok := classes[value]; ok {
		value = expanded
	}
	value = strings.ReplaceAll(value, `\n`, "\n")
	value = strings.ReplaceAll(value, `\t`, "\t")
	runes := []rune(value)
	var out []rune
	for i := 0; i < len(runes); i++ {
		if i+2 < len(runes) && runes[i+1] == '-' && runes[i] <= runes[i+2] {
			for r := runes[i]; r <= runes[i+2]; r++ {
				out = append(out, r)
			}
			i += 2
			continue
		}
		out = append(out, runes[i])
	}
	return out
}
