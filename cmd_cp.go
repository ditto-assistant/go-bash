package gobash

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/spf13/afero"
)

func init() { Register("cp", cmdCp) }

func cmdCp(_ context.Context, e *Env) int {
	flags, operands := splitArgs(e.Args[1:])
	if !validateShortFlags(e, flags, "rRn") {
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
		target := destination
		if destIsDir {
			target = path.Join(destination, path.Base(source))
		}
		if err := copyVirtualPath(e, e.Resolve(source), target, flags['r'] || flags['R'], flags['n']); err != nil {
			e.Errorf("cannot copy '%s': %v", source, err)
			code = 1
		}
	}
	return code
}

func copyVirtualPath(e *Env, source, target string, recursive, noClobber bool) error {
	info, err := e.FS.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		if exists, _ := afero.Exists(e.FS, target); noClobber && exists {
			return nil
		}
		data, err := afero.ReadFile(e.FS, source)
		if err != nil {
			return err
		}
		return afero.WriteFile(e.FS, target, data, info.Mode())
	}
	if !recursive {
		return fmt.Errorf("omitting directory (use -r)")
	}
	if target == source || strings.HasPrefix(target, strings.TrimRight(source, "/")+"/") {
		return fmt.Errorf("cannot copy a directory into itself")
	}
	return afero.Walk(e.FS, source, func(current string, itemInfo fs.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(current, source), "/")
		dest := target
		if rel != "" {
			dest = path.Join(target, rel)
		}
		if itemInfo.IsDir() {
			return e.FS.MkdirAll(dest, itemInfo.Mode().Perm())
		}
		if exists, _ := afero.Exists(e.FS, dest); noClobber && exists {
			return nil
		}
		data, err := afero.ReadFile(e.FS, current)
		if err != nil {
			return err
		}
		return afero.WriteFile(e.FS, dest, data, itemInfo.Mode().Perm())
	})
}
