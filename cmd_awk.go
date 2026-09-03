package gobash

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"strings"

	"github.com/benhoyt/goawk/interp"
	"github.com/benhoyt/goawk/parser"
)

func init() { Register("awk", cmdAwk) }

func cmdAwk(ctx context.Context, e *Env) int {
	program, vars, operands, ok := parseAwkArgs(e)
	if !ok {
		return 2
	}
	if unsafeAwkControl.MatchString(program) {
		e.Errorf("explicit loops and user-defined functions are disabled in the bounded sandbox")
		return 2
	}
	parsed, err := parser.ParseProgram([]byte(program), nil)
	if err != nil {
		e.Errorf("%v", err)
		return 2
	}
	var input bytes.Buffer
	code := forEachInput(ctx, e, operands, func(_ context.Context, _ string, r io.Reader) error {
		_, err := io.Copy(&input, r)
		return err
	})
	if code != 0 {
		return code
	}
	if err := ctx.Err(); err != nil {
		e.Errorf("%v", err)
		return 1
	}
	status, err := interp.ExecProgram(parsed, &interp.Config{
		Stdin: bytes.NewReader(input.Bytes()), Output: e.Stdout, Error: e.Stderr,
		Argv0: "awk", Vars: vars, NoExec: true, NoFileReads: true,
		NoFileWrites: true, Environ: []string{}, Chars: true,
	})
	if err != nil {
		e.Errorf("%v", err)
		return 2
	}
	return status
}

// goawk has no execution-step or context-cancellation hook. The useful
// record-filtering subset is safe, but explicit loops and recursive functions
// could pin a process forever, so reject those constructs before execution.
var unsafeAwkControl = regexp.MustCompile(`\b(?:while|for|do|function)\b`)

func parseAwkArgs(e *Env) (program string, vars, operands []string, ok bool) {
	args := e.Args[1:]
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-F" || a == "-v" {
			if i+1 >= len(args) {
				e.Errorf("option %s requires an argument", a)
				return "", nil, nil, false
			}
			i++
			if a == "-F" {
				vars = append(vars, "FS", args[i])
			} else {
				parts := strings.SplitN(args[i], "=", 2)
				if len(parts) != 2 {
					e.Errorf("invalid variable assignment: %s", args[i])
					return "", nil, nil, false
				}
				vars = append(vars, parts[0], parts[1])
			}
			continue
		}
		if strings.HasPrefix(a, "-F") && len(a) > 2 {
			vars = append(vars, "FS", strings.TrimPrefix(a, "-F"))
			continue
		}
		if program == "" {
			program = a
		} else {
			operands = append(operands, a)
		}
	}
	if program == "" {
		e.Errorf("missing program")
		return "", nil, nil, false
	}
	return program, vars, operands, true
}
