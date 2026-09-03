package gobash

import (
	"context"
	"fmt"
	"path"
	"strings"
)

func init() {
	Register("basename", cmdBasename)
	Register("dirname", cmdDirname)
}

func cmdBasename(_ context.Context, e *Env) int {
	if len(e.Args) < 2 {
		e.Errorf("missing operand")
		return 1
	}
	name := path.Base(strings.TrimRight(e.Args[1], "/"))
	if len(e.Args) > 2 {
		name = strings.TrimSuffix(name, e.Args[2])
	}
	fmt.Fprintln(e.Stdout, name)
	return 0
}

func cmdDirname(_ context.Context, e *Env) int {
	if len(e.Args) < 2 {
		e.Errorf("missing operand")
		return 1
	}
	fmt.Fprintln(e.Stdout, path.Dir(e.Args[1]))
	return 0
}
