package gobash

import (
	"context"
	"io"
)

func init() { Register("cat", cmdCat) }

// cmdCat concatenates files (or stdin when given no operands or "-") to stdout.
func cmdCat(_ context.Context, e *Env) int {
	operands := e.Args[1:]
	if len(operands) == 0 {
		_, _ = io.Copy(e.Stdout, e.Stdin)
		return 0
	}
	code := 0
	for _, name := range operands {
		if name == "-" {
			_, _ = io.Copy(e.Stdout, e.Stdin)
			continue
		}
		f, err := e.FS.Open(e.Resolve(name))
		if err != nil {
			e.Errorf("%s: No such file or directory", name)
			code = 1
			continue
		}
		_, _ = io.Copy(e.Stdout, f)
		_ = f.Close()
	}
	return code
}
