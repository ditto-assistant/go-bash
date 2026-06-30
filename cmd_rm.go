package gobash

import (
	"context"

	"github.com/spf13/afero"
)

func init() { Register("rm", cmdRm) }

// cmdRm removes files and, with -r, directories recursively. The -f flag
// suppresses errors about missing files.
func cmdRm(_ context.Context, e *Env) int {
	flags, operands := splitArgs(e.Args[1:])
	recursive := flags['r'] || flags['R']
	force := flags['f']
	if len(operands) == 0 {
		if force {
			return 0
		}
		e.Errorf("missing operand")
		return 1
	}
	code := 0
	for _, name := range operands {
		full := e.Resolve(name)
		exists, _ := afero.Exists(e.FS, full)
		if !exists {
			if !force {
				e.Errorf("cannot remove '%s': No such file or directory", name)
				code = 1
			}
			continue
		}
		isDir, _ := afero.IsDir(e.FS, full)
		if isDir && !recursive {
			e.Errorf("cannot remove '%s': Is a directory", name)
			code = 1
			continue
		}
		var err error
		if recursive {
			err = e.FS.RemoveAll(full)
		} else {
			err = e.FS.Remove(full)
		}
		if err != nil && !force {
			e.Errorf("cannot remove '%s': %v", name, err)
			code = 1
		}
	}
	return code
}
