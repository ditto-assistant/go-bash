package gobash

import (
	"bufio"
	"context"
	"errors"
	"io"
)

// forEachInput opens each operand from the virtual filesystem, using stdin for
// an empty operand list or "-". The callback must consume the reader before it
// returns; files are closed immediately afterwards.
func forEachInput(ctx context.Context, e *Env, operands []string, fn func(context.Context, string, io.Reader) error) int {
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	code := 0
	for _, name := range operands {
		if err := ctx.Err(); err != nil {
			e.Errorf("%v", err)
			return 1
		}
		if name == "-" {
			if err := fn(ctx, "-", e.Stdin); err != nil {
				e.Errorf("-: %v", err)
				code = 1
			}
			continue
		}
		f, err := e.FS.Open(e.Resolve(name))
		if err != nil {
			e.Errorf("%s: %v", name, err)
			code = 1
			continue
		}
		err = fn(ctx, name, f)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			e.Errorf("%s: %v", name, errors.Join(err, closeErr))
			code = 1
		}
	}
	return code
}

func scanLines(ctx context.Context, r io.Reader, fn func(line string, lineNo int) error) error {
	scanner := bufio.NewScanner(r)
	// Tool payloads frequently contain minified JSON with very long lines.
	scanner.Buffer(make([]byte, 64*1024), 32*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		lineNo++
		if err := fn(scanner.Text(), lineNo); err != nil {
			return err
		}
	}
	return scanner.Err()
}
