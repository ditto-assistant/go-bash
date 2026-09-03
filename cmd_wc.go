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
		printWC(e, count, name, showLines, showWords, showBytes, showChars, len(operands) > 0)
		total.lines += count.lines
		total.words += count.words
		total.bytes += count.bytes
		total.chars += count.chars
		inputs++
		return nil
	})
	if inputs > 1 {
		printWC(e, total, "total", showLines, showWords, showBytes, showChars, true)
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

func printWC(e *Env, count wcCount, name string, lines, words, bytes, chars, includeName bool) {
	if lines {
		fmt.Fprintf(e.Stdout, "%d ", count.lines)
	}
	if words {
		fmt.Fprintf(e.Stdout, "%d ", count.words)
	}
	if bytes {
		fmt.Fprintf(e.Stdout, "%d ", count.bytes)
	}
	if chars {
		fmt.Fprintf(e.Stdout, "%d ", count.chars)
	}
	if includeName {
		fmt.Fprint(e.Stdout, name)
	}
	fmt.Fprintln(e.Stdout)
}
