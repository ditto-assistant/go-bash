package gobash

import (
	"context"
	"time"

	"github.com/spf13/afero"
)

func init() { Register("touch", cmdTouch) }

// cmdTouch creates empty files that do not exist and updates the modification
// time of files that do.
func cmdTouch(_ context.Context, e *Env) int {
	_, operands := splitArgs(e.Args[1:])
	if len(operands) == 0 {
		e.Errorf("missing file operand")
		return 1
	}
	code := 0
	now := time.Now()
	for _, name := range operands {
		full := e.Resolve(name)
		if exists, _ := afero.Exists(e.FS, full); exists {
			if err := e.FS.Chtimes(full, now, now); err != nil {
				e.Errorf("cannot touch '%s': %v", name, err)
				code = 1
			}
			continue
		}
		f, err := e.FS.Create(full)
		if err != nil {
			e.Errorf("cannot touch '%s': %v", name, err)
			code = 1
			continue
		}
		_ = f.Close()
	}
	return code
}
