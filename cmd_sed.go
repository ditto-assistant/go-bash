package gobash

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

func init() { Register("sed", cmdSed) }

type sedProgram struct {
	addressFrom int
	addressTo   int
	pattern     *regexp.Regexp
	replacement string
	global      bool
	printOnly   bool
	deleteOnly  bool
}

func cmdSed(ctx context.Context, e *Env) int {
	expressions, quiet, extended, operands, ok := parseSedArgs(e)
	if !ok {
		return 2
	}
	programs := make([]sedProgram, 0, len(expressions))
	for _, expression := range expressions {
		program, err := compileSed(expression, extended)
		if err != nil {
			e.Errorf("%v", err)
			return 1
		}
		programs = append(programs, program)
	}
	return forEachInput(ctx, e, operands, func(ctx context.Context, _ string, r io.Reader) error {
		return scanLines(ctx, r, func(line string, lineNo int) error {
			deleted := false
			for _, program := range programs {
				addressed := program.addressFrom == 0 || lineNo >= program.addressFrom && lineNo <= program.addressTo
				if !addressed {
					continue
				}
				if program.deleteOnly {
					deleted = true
					break
				}
				if program.pattern != nil {
					if program.global {
						line = program.pattern.ReplaceAllString(line, program.replacement)
					} else if location := program.pattern.FindStringIndex(line); location != nil {
						line = line[:location[0]] + program.pattern.ReplaceAllString(line[location[0]:location[1]], program.replacement) + line[location[1]:]
					}
				}
				if program.printOnly {
					if _, err := fmt.Fprintln(e.Stdout, line); err != nil {
						return err
					}
				}
			}
			if !deleted && !quiet {
				_, err := fmt.Fprintln(e.Stdout, line)
				return err
			}
			return nil
		})
	})
}

func parseSedArgs(e *Env) (expressions []string, quiet, extended bool, operands []string, ok bool) {
	args := e.Args[1:]
	endOfFlags := false
	for i := 0; i < len(args); i++ {
		if endOfFlags {
			operands = append(operands, args[i])
			continue
		}
		switch args[i] {
		case "--":
			endOfFlags = true
		case "-n":
			quiet = true
		case "-E", "-r":
			extended = true
		case "-e":
			if i+1 >= len(args) {
				e.Errorf("-e requires an expression")
				return nil, false, false, nil, false
			}
			i++
			expressions = append(expressions, args[i])
		default:
			if strings.HasPrefix(args[i], "-") && len(expressions) == 0 {
				e.Errorf("unsupported option: %s", args[i])
				return nil, false, false, nil, false
			}
			if len(expressions) == 0 {
				expressions = append(expressions, args[i])
			} else {
				operands = append(operands, args[i])
			}
		}
	}
	if len(expressions) == 0 {
		e.Errorf("missing script")
		return nil, false, false, nil, false
	}
	return expressions, quiet, extended, operands, true
}

func compileSed(expression string, extended bool) (sedProgram, error) {
	program := sedProgram{}
	command := expression
	if comma := strings.Index(command, ","); comma > 0 {
		from, err1 := strconv.Atoi(command[:comma])
		i := comma + 1
		for i < len(command) && command[i] >= '0' && command[i] <= '9' {
			i++
		}
		to, err2 := strconv.Atoi(command[comma+1 : i])
		if err1 == nil && err2 == nil {
			program.addressFrom, program.addressTo = from, to
			command = command[i:]
		}
	} else {
		i := 0
		for i < len(command) && command[i] >= '0' && command[i] <= '9' {
			i++
		}
		if i > 0 {
			n, _ := strconv.Atoi(command[:i])
			program.addressFrom, program.addressTo = n, n
			command = command[i:]
		}
	}
	switch command {
	case "p":
		program.printOnly = true
		return program, nil
	case "d":
		program.deleteOnly = true
		return program, nil
	}
	if len(command) < 4 || command[0] != 's' {
		return program, fmt.Errorf("unsupported script: %s", expression)
	}
	delim := command[1]
	parts := splitSed(command[2:], delim)
	if len(parts) < 3 {
		return program, fmt.Errorf("unterminated substitute command")
	}
	pattern := parts[0]
	if !extended {
		pattern = sedBREToGo(pattern)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return program, err
	}
	program.pattern = re
	program.replacement = sedReplacementToGo(parts[1])
	program.global = strings.Contains(parts[2], "g")
	program.printOnly = strings.Contains(parts[2], "p")
	return program, nil
}

func sedBREToGo(pattern string) string {
	var out strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' && i+1 < len(pattern) {
			i++
			if strings.ContainsRune("(){}+?|", rune(pattern[i])) {
				out.WriteByte(pattern[i])
			} else {
				out.WriteByte('\\')
				out.WriteByte(pattern[i])
			}
			continue
		}
		if strings.ContainsRune("(){}+?|", rune(pattern[i])) {
			out.WriteByte('\\')
		}
		out.WriteByte(pattern[i])
	}
	return out.String()
}

func sedReplacementToGo(replacement string) string {
	var out strings.Builder
	for i := 0; i < len(replacement); i++ {
		switch replacement[i] {
		case '\\':
			if i+1 >= len(replacement) {
				out.WriteString(`\\`)
				continue
			}
			i++
			next := replacement[i]
			if next >= '0' && next <= '9' {
				fmt.Fprintf(&out, "${%c}", next)
			} else if next == '&' {
				out.WriteByte('&')
			} else {
				out.WriteByte(next)
			}
		case '&':
			out.WriteString("${0}")
		case '$':
			out.WriteString("$$")
		default:
			out.WriteByte(replacement[i])
		}
	}
	return out.String()
}

func splitSed(value string, delimiter byte) []string {
	parts := []string{""}
	escaped := false
	for i := 0; i < len(value); i++ {
		c := value[i]
		if escaped {
			if c != delimiter {
				parts[len(parts)-1] += "\\"
			}
			parts[len(parts)-1] += string(c)
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == delimiter {
			parts = append(parts, "")
			continue
		}
		parts[len(parts)-1] += string(c)
	}
	return parts
}
