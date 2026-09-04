package gobash

import (
	"context"
	"io"
	"strings"
)

func init() { Register("cat", cmdCat) }

// cmdCat concatenates files (or stdin when given no operands or "-") to stdout.
func cmdCat(_ context.Context, e *Env) int {
	operands := make([]string, 0, len(e.Args)-1)
	endOfFlags := false
	for _, arg := range e.Args[1:] {
		if !endOfFlags && arg == "--" {
			endOfFlags = true
			continue
		}
		if !endOfFlags && strings.HasPrefix(arg, "-") && arg != "-" {
			e.Errorf("unsupported option: %s", arg)
			return 2
		}
		operands = append(operands, arg)
	}
	if len(operands) == 0 {
		if _, err := io.Copy(e.Stdout, e.Stdin); err != nil {
			e.Errorf("%v", err)
			return 1
		}
		return 0
	}
	code := 0
	for _, name := range operands {
		if name == "-" {
			if _, err := io.Copy(e.Stdout, e.Stdin); err != nil {
				e.Errorf("%v", err)
				code = 1
			}
			continue
		}
		f, err := e.FS.Open(e.Resolve(name))
		if err != nil {
			e.Errorf("%s: No such file or directory", name)
			code = 1
			continue
		}
		if _, err := io.Copy(e.Stdout, f); err != nil {
			e.Errorf("%s: %v", name, err)
			code = 1
		}
		if err := f.Close(); err != nil {
			e.Errorf("%s: %v", name, err)
			code = 1
		}
	}
	return code
}
