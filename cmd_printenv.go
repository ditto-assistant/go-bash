package gobash

import (
	"context"
	"fmt"
	"strings"
)

func init() { Register("printenv", cmdPrintenv) }

func cmdPrintenv(_ context.Context, e *Env) int {
	if len(e.Args) == 1 {
		return cmdEnv(context.Background(), &Env{Args: []string{"env"}, Stdout: e.Stdout, Stderr: e.Stderr, Environ: e.Environ})
	}
	values := make(map[string]string, len(e.Environ))
	for _, pair := range e.Environ {
		name, value, _ := strings.Cut(pair, "=")
		values[name] = value
	}
	code := 0
	for _, name := range e.Args[1:] {
		value, ok := values[name]
		if !ok {
			code = 1
			continue
		}
		if _, err := fmt.Fprintln(e.Stdout, value); err != nil {
			e.Errorf("%v", err)
			return 1
		}
	}
	return code
}
