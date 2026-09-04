package gobash

import (
	"bufio"
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/spf13/afero"
)

func init() { Register("rm", cmdRm) }

// cmdRm removes files and, with -r, directories recursively. The -f flag
// suppresses errors about missing files.
func cmdRm(_ context.Context, e *Env) int {
	flags, operands := splitArgs(e.Args[1:])
	if !validateShortFlags(e, flags, "rRfi") {
		return 2
	}
	recursive := flags['r'] || flags['R']
	force := flags['f']
	interactive := flags['i']
	if len(operands) == 0 {
		if force {
			return 0
		}
		e.Errorf("missing operand")
		return 1
	}
	code := 0
	reader := bufio.NewReader(e.Stdin)
	for _, name := range operands {
		cleanOperand := path.Clean(name)
		if cleanOperand == "/" {
			e.Errorf("it is dangerous to operate recursively on '/'")
			code = 1
			continue
		}
		rawBase := path.Base(strings.TrimRight(name, "/"))
		if name == "" || rawBase == "." || rawBase == ".." || cleanOperand == "." || cleanOperand == ".." {
			e.Errorf("refusing to remove '.' or '..' directory: skipping '%s'", name)
			code = 1
			continue
		}
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
		if interactive {
			_, _ = fmt.Fprintf(e.Stderr, "rm: remove '%s'? ", name)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				continue
			}
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
