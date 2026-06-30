package gobash

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/afero"
)

func init() { Register("ls", cmdLs) }

// cmdLs lists directory contents (one entry per line, sorted). Supported flags:
// -a includes dotfiles. Output matches `ls -1` semantics, which is the natural
// form when piped — the common case for agents.
func cmdLs(_ context.Context, e *Env) int {
	flags, operands := splitArgs(e.Args[1:])
	showAll := flags['a']
	if len(operands) == 0 {
		operands = []string{"."}
	}
	code := 0
	for _, p := range operands {
		full := e.Resolve(p)
		fi, err := e.FS.Stat(full)
		if err != nil {
			e.Errorf("cannot access '%s': No such file or directory", p)
			code = 1
			continue
		}
		if !fi.IsDir() {
			fmt.Fprintln(e.Stdout, p)
			continue
		}
		infos, err := afero.ReadDir(e.FS, full)
		if err != nil {
			e.Errorf("cannot open directory '%s': %v", p, err)
			code = 1
			continue
		}
		names := make([]string, 0, len(infos))
		for _, info := range infos {
			if !showAll && strings.HasPrefix(info.Name(), ".") {
				continue
			}
			names = append(names, info.Name())
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintln(e.Stdout, n)
		}
	}
	return code
}
