package gobash

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/afero"
)

func init() { Register("ls", cmdLs) }

// cmdLs lists directory contents (one entry per line, sorted). Supported flags:
// -a includes dotfiles, -d lists directory operands themselves, and -l emits a
// stable long form with mode, owner/group, byte size, UTC time, and name.
// One-entry-per-line output is the default because it is the natural form when
// piped by agents.
func cmdLs(_ context.Context, e *Env) int {
	if len(e.Args) == 2 && (e.Args[1] == "--help" || e.Args[1] == "-h") {
		_, _ = fmt.Fprintln(e.Stdout, "usage: ls [-a1dl] [file...]")
		return 0
	}
	flags, operands := splitArgs(e.Args[1:])
	if !validateShortFlags(e, flags, "a1dl") {
		return 2
	}
	showAll := flags['a']
	listDirectories := !flags['d']
	long := flags['l']
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
		if !fi.IsDir() || !listDirectories {
			if err := writeLsEntry(e, p, fi, long); err != nil {
				e.Errorf("%v", err)
				code = 1
			}
			continue
		}
		infos, err := afero.ReadDir(e.FS, full)
		if err != nil {
			e.Errorf("cannot open directory '%s': %v", p, err)
			code = 1
			continue
		}
		entries := make([]os.FileInfo, 0, len(infos))
		for _, info := range infos {
			if !showAll && strings.HasPrefix(info.Name(), ".") {
				continue
			}
			entries = append(entries, info)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, info := range entries {
			if err := writeLsEntry(e, info.Name(), info, long); err != nil {
				e.Errorf("%v", err)
				code = 1
				break
			}
		}
	}
	return code
}

func writeLsEntry(e *Env, name string, info os.FileInfo, long bool) error {
	if !long {
		_, err := fmt.Fprintln(e.Stdout, name)
		return err
	}
	_, err := fmt.Fprintf(e.Stdout, "%s 1 agent agent %d %s %s\n",
		info.Mode().String(), info.Size(), info.ModTime().UTC().Format("Jan _2 15:04"), name)
	return err
}
