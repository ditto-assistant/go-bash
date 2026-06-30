package gobash

import "context"

func init() { Register("mkdir", cmdMkdir) }

// cmdMkdir creates directories. The -p flag creates parents as needed and does
// not error if the target already exists.
func cmdMkdir(_ context.Context, e *Env) int {
	flags, operands := splitArgs(e.Args[1:])
	parents := flags['p']
	if len(operands) == 0 {
		e.Errorf("missing operand")
		return 1
	}
	code := 0
	for _, d := range operands {
		full := e.Resolve(d)
		var err error
		if parents {
			err = e.FS.MkdirAll(full, 0o755)
		} else {
			err = e.FS.Mkdir(full, 0o755)
		}
		if err != nil {
			e.Errorf("cannot create directory '%s': %v", d, err)
			code = 1
		}
	}
	return code
}
