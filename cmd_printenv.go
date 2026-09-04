package gobash

import (
	"context"
	"fmt"
	"strings"
)

func init() { Register("printenv", cmdPrintenv) }

func cmdPrintenv(_ context.Context, e *Env) int {
	separator := "\n"
	args := e.Args[1:]
	if len(args) > 0 && (args[0] == "-0" || args[0] == "--null") {
		separator = "\x00"
		args = args[1:]
	}
	if len(args) == 0 {
		if separator == "\x00" {
			return cmdEnv(context.Background(), &Env{Args: []string{"env", "-0"}, Stdout: e.Stdout, Stderr: e.Stderr, Environ: e.Environ})
		}
		return cmdEnv(context.Background(), &Env{Args: []string{"env"}, Stdout: e.Stdout, Stderr: e.Stderr, Environ: e.Environ})
	}
	values := make(map[string]string, len(e.Environ))
	for _, pair := range e.Environ {
		name, value, _ := strings.Cut(pair, "=")
		values[name] = value
	}
	code := 0
	for _, name := range args {
		value, ok := values[name]
		if !ok {
			code = 1
			continue
		}
		if _, err := fmt.Fprint(e.Stdout, value+separator); err != nil {
			e.Errorf("%v", err)
			return 1
		}
	}
	return code
}
