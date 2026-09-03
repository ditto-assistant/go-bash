package gobash

import (
	"context"
	"path"

	"github.com/spf13/afero"
)

func init() { Register("mv", cmdMv) }

func cmdMv(_ context.Context, e *Env) int {
	_, operands := splitArgs(e.Args[1:])
	if len(operands) < 2 {
		e.Errorf("missing destination file operand")
		return 1
	}
	destination := e.Resolve(operands[len(operands)-1])
	sources := operands[:len(operands)-1]
	destIsDir, _ := afero.IsDir(e.FS, destination)
	if len(sources) > 1 && !destIsDir {
		e.Errorf("target '%s' is not a directory", operands[len(operands)-1])
		return 1
	}
	code := 0
	for _, source := range sources {
		target := destination
		if destIsDir {
			target = path.Join(destination, path.Base(source))
		}
		if err := e.FS.Rename(e.Resolve(source), target); err != nil {
			e.Errorf("cannot move '%s': %v", source, err)
			code = 1
		}
	}
	return code
}
