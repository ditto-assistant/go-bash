package gobash

import (
	"context"
	"fmt"
	"io"
	"unicode/utf8"
)

func init() { Register("wc", cmdWc) }

type wcCount struct {
	lines int
	words int
	bytes int
	chars int
}

func cmdWc(ctx context.Context, e *Env) int {
	flags, operands := splitArgs(e.Args[1:])
	if !validateShortFlags(e, flags, "lwcm") {
		return 2
	}
	showLines, showWords := flags['l'], flags['w']
	showBytes, showChars := flags['c'], flags['m']
	if !showLines && !showWords && !showBytes && !showChars {
		showLines, showWords, showBytes = true, true, true
	}
	var total wcCount
	inputs := 0
	code := forEachInput(ctx, e, operands, func(ctx context.Context, name string, r io.Reader) error {
		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		count := countWC(data)
		if err := printWC(e, count, name, showLines, showWords, showBytes, showChars, len(operands) > 0); err != nil {
			return err
		}
		total.lines += count.lines
		total.words += count.words
		total.bytes += count.bytes
		total.chars += count.chars
		inputs++
		return nil
	})
	if inputs > 1 {
		if err := printWC(e, total, "total", showLines, showWords, showBytes, showChars, true); err != nil {
			e.Errorf("%v", err)
			return 1
		}
	}
	return code
}

func countWC(data []byte) wcCount {
	count := wcCount{bytes: len(data), chars: utf8.RuneCount(data)}
	inWord := false
	for _, r := range string(data) {
		if r == '\n' {
			count.lines++
		}
		space := r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v'
		if space {
			inWord = false
		} else if !inWord {
			count.words++
			inWord = true
		}
	}
	return count
}

func printWC(e *Env, count wcCount, name string, lines, words, bytes, chars, includeName bool) error {
	if lines {
		if _, err := fmt.Fprintf(e.Stdout, "%d ", count.lines); err != nil {
			return err
		}
	}
	if words {
		if _, err := fmt.Fprintf(e.Stdout, "%d ", count.words); err != nil {
			return err
		}
	}
	if bytes {
		if _, err := fmt.Fprintf(e.Stdout, "%d ", count.bytes); err != nil {
			return err
		}
	}
	if chars {
		if _, err := fmt.Fprintf(e.Stdout, "%d ", count.chars); err != nil {
			return err
		}
	}
	if includeName {
		if _, err := fmt.Fprint(e.Stdout, name); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(e.Stdout)
	return err
}
