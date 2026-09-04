package gobash

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"mvdan.cc/sh/v3/syntax"
)

// gobash_quote is an internal helper for the printf compatibility shim. It
// implements the common `%q` and `%q\n` forms without exposing host execution.
func init() {
	registerInternal("gobash_quote", cmdGobashQuote)
	registerInternal("gobash_printf", cmdGobashPrintf)
	registerInternal("gobash_array_length", cmdGobashArrayLength)
}

// rewriteExplicitBuiltinPrintf routes `command printf` and `builtin printf`
// through the compatibility implementation. Without this small AST rewrite,
// those explicit forms bypass the prelude function and expose mvdan/sh's much
// smaller printf subset.
func rewriteExplicitBuiltinPrintf(file *syntax.File) {
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		first, second := call.Args[0].Lit(), call.Args[1].Lit()
		if (first != "command" && first != "builtin") || second != "printf" || len(call.Args[1].Parts) != 1 {
			return true
		}
		if lit, ok := call.Args[1].Parts[0].(*syntax.Lit); ok {
			lit.Value = "gobash_printf"
			call.Args = call.Args[1:]
		}
		return true
	})
}

// rewriteArrayLengths works around an upstream associative-array length bug
// while preserving the normal expansion shape, including inside double quotes.
func rewriteArrayLengths(file *syntax.File) {
	syntax.Walk(file, func(node syntax.Node) bool {
		var parts *[]syntax.WordPart
		switch node := node.(type) {
		case *syntax.Word:
			parts = &node.Parts
		case *syntax.DblQuoted:
			parts = &node.Parts
		default:
			return true
		}
		for i, part := range *parts {
			param, ok := part.(*syntax.ParamExp)
			if !ok || !param.Length || param.Param == nil || param.Index == nil {
				continue
			}
			index, ok := param.Index.(*syntax.Word)
			if !ok || index.Lit() != "@" && index.Lit() != "*" {
				continue
			}
			helper, err := syntax.NewParser().Parse(strings.NewReader("command gobash_array_length \"${!"+param.Param.Value+"[@]}\""), "gobash-array-length")
			if err == nil {
				(*parts)[i] = &syntax.CmdSubst{Stmts: helper.Stmts}
			}
		}
		return true
	})
}

func cmdGobashArrayLength(_ context.Context, e *Env) int {
	_, err := fmt.Fprint(e.Stdout, len(e.Args)-1)
	if err != nil {
		e.Errorf("%v", err)
		return 1
	}
	return 0
}

func cmdGobashQuote(_ context.Context, e *Env) int {
	if len(e.Args) < 2 || e.Args[1] != "%q" && e.Args[1] != `%q\n` {
		e.Errorf("supported formats are %%q and %%q\\n")
		return 2
	}
	values := e.Args[2:]
	if len(values) == 0 {
		values = []string{""}
	}
	newline := e.Args[1] == `%q\n`
	for _, value := range values {
		if _, err := fmt.Fprint(e.Stdout, bashQuoted(value)); err != nil {
			e.Errorf("%v", err)
			return 1
		}
		if newline {
			if _, err := fmt.Fprintln(e.Stdout); err != nil {
				e.Errorf("%v", err)
				return 1
			}
		}
	}
	return 0
}

func cmdGobashPrintf(_ context.Context, e *Env) int {
	args := e.Args[1:]
	endOfOptions := false
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
		endOfOptions = true
	}
	if len(args) == 0 {
		e.Errorf("usage: printf [-v var] format [arguments]")
		return 2
	}
	if args[0] == "-v" {
		e.Errorf("-v is only available through the shell printf builtin")
		return 2
	}
	if !endOfOptions && strings.HasPrefix(args[0], "-") && args[0] != "-" {
		e.Errorf("invalid option: %s", args[0])
		return 2
	}
	out, err := renderBashPrintf(args[0], args[1:])
	if err != nil {
		e.Errorf("%v", err)
		return 1
	}
	if _, err := fmt.Fprint(e.Stdout, out); err != nil {
		e.Errorf("%v", err)
		return 1
	}
	return 0
}

func renderBashPrintf(format string, args []string) (string, error) {
	var out strings.Builder
	argAt := 0
	for {
		consumedAtStart := argAt
		for i := 0; i < len(format); {
			if format[i] == '\\' {
				value, used := decodePrintfEscape(format[i:])
				out.WriteString(value)
				i += used
				continue
			}
			if format[i] != '%' {
				out.WriteByte(format[i])
				i++
				continue
			}
			start := i
			i++
			if i < len(format) && format[i] == '%' {
				out.WriteByte('%')
				i++
				continue
			}
			for i < len(format) && strings.ContainsRune("-+ #0", rune(format[i])) {
				i++
			}
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
			if i < len(format) && format[i] == '.' {
				i++
				for i < len(format) && format[i] >= '0' && format[i] <= '9' {
					i++
				}
			}
			if i >= len(format) {
				return "", fmt.Errorf("missing format character")
			}
			verb := format[i]
			i++
			directive := format[start:i]
			value := ""
			if argAt < len(args) {
				value = args[argAt]
				argAt++
			}
			switch verb {
			case 's':
				_, _ = fmt.Fprintf(&out, directive, value)
			case 'q':
				_, _ = fmt.Fprintf(&out, directive[:len(directive)-1]+"s", bashQuoted(value))
			case 'b':
				decoded, stop := decodePrintfBytes(value)
				_, _ = fmt.Fprintf(&out, directive[:len(directive)-1]+"s", decoded)
				if stop {
					return out.String(), nil
				}
			case 'c':
				r := rune(0)
				if value != "" {
					r, _ = utf8FirstRune(value)
				}
				out.WriteString(fmt.Sprintf(directive, r))
			case 'd', 'i', 'o', 'x', 'X', 'u':
				n, err := parsePrintfInteger(value)
				if err != nil {
					return "", err
				}
				goDirective := directive
				if verb == 'i' {
					goDirective = directive[:len(directive)-1] + "d"
				}
				if verb == 'u' {
					out.WriteString(fmt.Sprintf(goDirective[:len(goDirective)-1]+"d", uint64(n)))
				} else {
					out.WriteString(fmt.Sprintf(goDirective, n))
				}
			case 'f', 'F', 'e', 'E', 'g', 'G':
				n, err := strconv.ParseFloat(zeroIfEmpty(value), 64)
				if err != nil {
					return "", fmt.Errorf("%s: invalid number", value)
				}
				out.WriteString(fmt.Sprintf(directive, n))
			default:
				return "", fmt.Errorf("unsupported format character: %%%c", verb)
			}
		}
		if argAt >= len(args) || argAt == consumedAtStart {
			break
		}
	}
	return out.String(), nil
}

func zeroIfEmpty(value string) string {
	if value == "" {
		return "0"
	}
	return value
}

func parsePrintfInteger(value string) (int64, error) {
	value = zeroIfEmpty(strings.TrimSpace(value))
	if strings.Contains(value, "#") {
		baseText, digits, ok := strings.Cut(value, "#")
		base, err := strconv.Atoi(baseText)
		if !ok || err != nil || base < 2 || base > 36 {
			return 0, fmt.Errorf("%s: invalid number", value)
		}
		n, err := strconv.ParseInt(digits, base, 64)
		if err != nil {
			return 0, fmt.Errorf("%s: invalid number", value)
		}
		return n, nil
	}
	n, err := strconv.ParseInt(value, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid number", value)
	}
	return n, nil
}

func decodePrintfBytes(value string) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(value); {
		if value[i] == '\\' && i+1 < len(value) && value[i+1] == 'c' {
			return out.String(), true
		}
		decoded, used := decodePrintfEscape(value[i:])
		out.WriteString(decoded)
		i += used
	}
	return out.String(), false
}

func decodePrintfEscape(value string) (string, int) {
	if len(value) < 2 || value[0] != '\\' {
		return value[:1], 1
	}
	switch value[1] {
	case 'a':
		return "\a", 2
	case 'b':
		return "\b", 2
	case 'e', 'E':
		return "\x1b", 2
	case 'f':
		return "\f", 2
	case 'n':
		return "\n", 2
	case 'r':
		return "\r", 2
	case 't':
		return "\t", 2
	case 'v':
		return "\v", 2
	case '\\':
		return "\\", 2
	case '\'', '"':
		return string(value[1]), 2
	case 'x':
		return decodePrintfDigits(value, 2, 2, 16)
	case 'u':
		return decodePrintfDigits(value, 2, 4, 16)
	case 'U':
		return decodePrintfDigits(value, 2, 8, 16)
	case '0':
		decoded, used := decodePrintfDigits(value, 2, 3, 8)
		if used == 2 && decoded == value[:2] {
			return "\x00", 2
		}
		return decoded, used
	default:
		return value[:2], 2
	}
}

func decodePrintfDigits(value string, start, maxDigits, base int) (string, int) {
	end := start
	for end < len(value) && end-start < maxDigits {
		c := value[end]
		valid := c >= '0' && c <= '9' || base == 16 && (c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F')
		if !valid || base == 8 && c > '7' {
			break
		}
		end++
	}
	if end == start {
		return value[:min(len(value), start)], min(len(value), start)
	}
	n, _ := strconv.ParseUint(value[start:end], base, 32)
	return string(rune(n)), end
}

func utf8FirstRune(value string) (rune, int) {
	for _, r := range value {
		return r, len(string(r))
	}
	return 0, 0
}

func bashQuoted(value string) string {
	if value == "" {
		return "''"
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			var out strings.Builder
			out.WriteString("$'")
			for _, q := range value {
				switch q {
				case '\n':
					out.WriteString(`\n`)
				case '\r':
					out.WriteString(`\r`)
				case '\t':
					out.WriteString(`\t`)
				case '\\', '\'':
					out.WriteByte('\\')
					out.WriteRune(q)
				default:
					if unicode.IsControl(q) {
						fmt.Fprintf(&out, `\x%02x`, q)
					} else {
						out.WriteRune(q)
					}
				}
			}
			out.WriteByte('\'')
			return out.String()
		}
	}
	var out strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_@%+=:,./-", r) {
			out.WriteRune(r)
		} else {
			out.WriteByte('\\')
			out.WriteRune(r)
		}
	}
	return out.String()
}
