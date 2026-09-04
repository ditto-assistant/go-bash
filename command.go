package gobash

import (
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"time"

	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/syntax"
)

// CommandFunc implements a single builtin command. It returns the process exit
// code (0 for success). Implementations must read input from env.Stdin and
// write to env.Stdout / env.Stderr; they must access the filesystem only
// through env.FS so that everything stays inside the virtual environment.
type CommandFunc func(ctx context.Context, env *Env) int

// Env is the execution context handed to a builtin for a single invocation.
type Env struct {
	// Args is the full argument vector; Args[0] is the command name.
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// FS is the shared virtual filesystem.
	FS afero.Fs
	// Dir is the current working directory (absolute).
	Dir string
	// Environ is the sorted exported environment for this invocation.
	Environ []string
	// Now returns the shell clock time.
	Now func() time.Time
	// RunCommand invokes one argv vector through the same shell-aware dispatcher.
	// It can resolve both interpreter builtins and registered external commands;
	// external invocations count against the command limit. It is used by
	// commands such as xargs.
	RunCommand func(context.Context, []string) int
}

// Resolve turns a possibly-relative path into a cleaned absolute path within
// the virtual filesystem, relative to the current working directory.
func (e *Env) Resolve(p string) string {
	if path.IsAbs(p) {
		return path.Clean(p)
	}
	return path.Clean(path.Join(e.Dir, p))
}

// Errorf writes a "<command>: <msg>" diagnostic to stderr.
func (e *Env) Errorf(format string, a ...any) {
	name := "gobash"
	if len(e.Args) > 0 {
		name = e.Args[0]
	}
	_, _ = fmt.Fprintf(e.Stderr, name+": "+format+"\n", a...)
}

// registry holds all builtins, populated by Register (typically from per-command
// init functions). A central map with self-registering command files keeps each
// command independent and merge-conflict free.
var registry = map[string]CommandFunc{}

// Register adds a builtin under name. It panics on duplicate registration so
// collisions surface at startup rather than silently shadowing.
func Register(name string, fn CommandFunc) {
	if !syntax.ValidName(name) {
		panic("gobash: command name is not a safe shell identifier: " + name)
	}
	if _, dup := registry[name]; dup {
		panic("gobash: duplicate command registration: " + name)
	}
	registry[name] = fn
}

// Commands returns the sorted names of all registered builtins.
func Commands() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func lookup(name string) (CommandFunc, bool) {
	fn, ok := registry[name]
	return fn, ok
}
