package gobash

import (
	"context"
	"io"
	"os"
)

func init() { Register("tee", cmdTee) }

func cmdTee(_ context.Context, e *Env) int {
	flags, operands := splitArgs(e.Args[1:])
	if !validateShortFlags(e, flags, "a") {
		return 2
	}
	writers := []io.Writer{e.Stdout}
	files := make([]io.Closer, 0, len(operands))
	openFlags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if flags['a'] {
		openFlags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	for _, operand := range operands {
		f, err := e.FS.OpenFile(e.Resolve(operand), openFlags, 0o644)
		if err != nil {
			e.Errorf("%s: %v", operand, err)
			for _, file := range files {
				_ = file.Close()
			}
			return 1
		}
		writers = append(writers, f)
		files = append(files, f)
	}
	_, err := io.Copy(io.MultiWriter(writers...), e.Stdin)
	for _, file := range files {
		_ = file.Close()
	}
	if err != nil {
		e.Errorf("%v", err)
		return 1
	}
	return 0
}
