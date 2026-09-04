package gobash

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/spf13/afero"
)

func init() { Register("mv", cmdMv) }

func cmdMv(_ context.Context, e *Env) int {
	flags, operands := splitArgs(e.Args[1:])
	if !validateShortFlags(e, flags, "n") {
		return 2
	}
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
		sourcePath := e.Resolve(source)
		target := destination
		if destIsDir {
			target = path.Join(destination, path.Base(source))
		}
		if info, err := e.FS.Stat(sourcePath); err == nil && info.IsDir() &&
			(target == sourcePath || strings.HasPrefix(target, strings.TrimRight(sourcePath, "/")+"/")) {
			e.Errorf("cannot move '%s': %v", source, fmt.Errorf("cannot move a directory into itself"))
			code = 1
			continue
		}
		if exists, _ := afero.Exists(e.FS, target); flags['n'] && exists {
			continue
		}
		if err := e.FS.Rename(sourcePath, target); err != nil {
			e.Errorf("cannot move '%s': %v", source, err)
			code = 1
		}
	}
	return code
}
