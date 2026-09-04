package gobash

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"

	"github.com/spf13/afero"
)

func init() { Register("find", cmdFind) }

type findOptions struct {
	root       string
	name       string
	ignoreCase bool
	typeFilter byte
	maxDepth   int
}

func cmdFind(ctx context.Context, e *Env) int {
	if len(e.Args) == 2 && e.Args[1] == "--help" {
		_, _ = fmt.Fprintln(e.Stdout, "usage: find [path] [-maxdepth n] [-type f|d] [-name pattern|-iname pattern] [-print]")
		return 0
	}
	opts, ok := parseFindOptions(e)
	if !ok {
		return 1
	}
	root := e.Resolve(opts.root)
	err := afero.Walk(e.FS, root, func(current string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(current, root), "/")
		depth := 0
		if rel != "" {
			depth = strings.Count(rel, "/") + 1
		}
		if opts.maxDepth >= 0 && depth > opts.maxDepth {
			if info.IsDir() {
				return filepathSkipDir
			}
			return nil
		}
		if opts.typeFilter == 'f' && info.IsDir() || opts.typeFilter == 'd' && !info.IsDir() {
			return nil
		}
		name, pattern := info.Name(), opts.name
		if opts.ignoreCase {
			name, pattern = strings.ToLower(name), strings.ToLower(pattern)
		}
		if pattern != "" {
			matched, matchErr := path.Match(pattern, name)
			if matchErr != nil {
				return matchErr
			}
			if !matched {
				return nil
			}
		}
		display := opts.root
		if rel != "" {
			display = strings.TrimRight(opts.root, "/") + "/" + rel
		}
		_, err = fmt.Fprintln(e.Stdout, display)
		return err
	})
	if err != nil {
		e.Errorf("%v", err)
		return 1
	}
	return 0
}

var filepathSkipDir = fs.SkipDir

func parseFindOptions(e *Env) (findOptions, bool) {
	opts := findOptions{root: ".", maxDepth: -1}
	args := e.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		opts.root = args[0]
		args = args[1:]
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-print":
			// Printing matching paths is already the default action. Accept the
			// explicit GNU/BSD spelling without consuming another argument.
			continue
		case "-name", "-iname", "-type", "-maxdepth":
			if i+1 >= len(args) {
				e.Errorf("predicate %s requires an argument", args[i])
				return opts, false
			}
		default:
			e.Errorf("unsupported predicate: %s", args[i])
			return opts, false
		}
		value := args[i+1]
		switch args[i] {
		case "-name":
			opts.name = value
		case "-iname":
			opts.name, opts.ignoreCase = value, true
		case "-type":
			if value != "f" && value != "d" {
				e.Errorf("unsupported file type: %s", value)
				return opts, false
			}
			opts.typeFilter = value[0]
		case "-maxdepth":
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				e.Errorf("invalid maxdepth: %s", value)
				return opts, false
			}
			opts.maxDepth = n
		}
		i++
	}
	return opts, true
}
