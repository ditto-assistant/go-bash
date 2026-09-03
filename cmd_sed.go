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
	quiet       bool
	addressFrom int
	addressTo   int
	pattern     *regexp.Regexp
	replacement string
	global      bool
	printOnly   bool
	deleteOnly  bool
}

func cmdSed(ctx context.Context, e *Env) int {
	expression, quiet, operands, ok := parseSedArgs(e)
	if !ok {
		return 2
	}
	program, err := compileSed(expression)
	if err != nil {
		e.Errorf("%v", err)
		return 1
	}
	program.quiet = quiet
	return forEachInput(ctx, e, operands, func(ctx context.Context, _ string, r io.Reader) error {
		return scanLines(ctx, r, func(line string, lineNo int) error {
			addressed := program.addressFrom == 0 || lineNo >= program.addressFrom && lineNo <= program.addressTo
			if !addressed {
				if !program.quiet {
					fmt.Fprintln(e.Stdout, line)
				}
				return nil
			}
			if program.deleteOnly {
				return nil
			}
			if program.pattern != nil {
				if program.global {
					line = program.pattern.ReplaceAllString(line, program.replacement)
				} else {
					location := program.pattern.FindStringIndex(line)
					if location != nil {
						line = line[:location[0]] + program.pattern.ReplaceAllString(line[location[0]:location[1]], program.replacement) + line[location[1]:]
					}
				}
			}
			if !program.quiet || program.printOnly {
				fmt.Fprintln(e.Stdout, line)
			}
			return nil
		})
	})
}

func parseSedArgs(e *Env) (expression string, quiet bool, operands []string, ok bool) {
	args := e.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-n":
			quiet = true
		case "-e":
			if i+1 >= len(args) || expression != "" {
				e.Errorf("-e requires one expression")
				return "", false, nil, false
			}
			i++
			expression = args[i]
		default:
			if expression == "" {
				expression = args[i]
			} else {
				operands = append(operands, args[i])
			}
		}
	}
	if expression == "" {
		e.Errorf("missing script")
		return "", false, nil, false
	}
	return expression, quiet, operands, true
}

func compileSed(expression string) (sedProgram, error) {
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
	re, err := regexp.Compile(parts[0])
	if err != nil {
		return program, err
	}
	program.pattern = re
	program.replacement = parts[1]
	program.global = strings.Contains(parts[2], "g")
	program.printOnly = strings.Contains(parts[2], "p")
	return program, nil
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
