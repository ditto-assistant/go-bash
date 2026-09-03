package gobash

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/spf13/afero"
)

func init() { Register("tree", cmdTree) }

func cmdTree(ctx context.Context, e *Env) int {
	flags, operands := splitArgs(e.Args[1:])
	if len(operands) == 0 {
		operands = []string{"."}
	}
	code := 0
	for _, operand := range operands {
		root := e.Resolve(operand)
		if _, err := e.FS.Stat(root); err != nil {
			e.Errorf("%s: %v", operand, err)
			code = 1
			continue
		}
		fmt.Fprintln(e.Stdout, operand)
		files, dirs := 0, 0
		if err := treeWalk(ctx, e, root, "", flags['a'], &files, &dirs); err != nil {
			e.Errorf("%s: %v", operand, err)
			code = 1
		}
		fmt.Fprintf(e.Stdout, "\n%d directories, %d files\n", dirs, files)
	}
	return code
}

func treeWalk(ctx context.Context, e *Env, dir, prefix string, all bool, files, dirs *int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := afero.ReadDir(e.FS, dir)
	if err != nil {
		return err
	}
	visible := entries[:0]
	for _, entry := range entries {
		if all || !strings.HasPrefix(entry.Name(), ".") {
			visible = append(visible, entry)
		}
	}
	for i, entry := range visible {
		last := i == len(visible)-1
		branch, childPrefix := "├── ", prefix+"│   "
		if last {
			branch, childPrefix = "└── ", prefix+"    "
		}
		fmt.Fprintln(e.Stdout, prefix+branch+entry.Name())
		if entry.IsDir() {
			*dirs++
			if err := treeWalk(ctx, e, path.Join(dir, entry.Name()), childPrefix, all, files, dirs); err != nil {
				return err
			}
		} else {
			*files++
		}
	}
	return nil
}
