package gobash

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

func init() {
	Register("gzip", cmdGzip)
	Register("gunzip", cmdGzip)
}

type gzipOptions struct {
	decompress bool
	stdout     bool
	keep       bool
	test       bool
	level      int
	operands   []string
}

// cmdGzip implements the agent-useful gzip/gunzip surface: stream mode,
// -c/--stdout, -d/--decompress, -k/--keep, -t/--test, and levels -1 through -9.
func cmdGzip(ctx context.Context, e *Env) int {
	if len(e.Args) == 2 && e.Args[1] == "--help" {
		_, _ = fmt.Fprintln(e.Stdout, "usage: gzip [-cdktf1-9] [file ...]")
		return 0
	}
	opts, ok := parseGzipArgs(e)
	if !ok {
		return 2
	}
	if e.Args[0] == "gunzip" {
		opts.decompress = true
	}
	if len(opts.operands) == 0 {
		if err := runGzipStream(ctx, e.Stdin, e.Stdout, opts); err != nil {
			e.Errorf("%v", err)
			return 1
		}
		return 0
	}
	code := 0
	for _, operand := range opts.operands {
		if err := ctx.Err(); err != nil {
			e.Errorf("%v", err)
			return 1
		}
		if err := runGzipFile(ctx, e, operand, opts); err != nil {
			e.Errorf("%s: %v", operand, err)
			code = 1
		}
	}
	return code
}

func runGzipFile(ctx context.Context, e *Env, operand string, opts gzipOptions) error {
	source, err := e.FS.Open(e.Resolve(operand))
	if err != nil {
		return err
	}

	if opts.stdout || opts.test {
		out := e.Stdout
		if opts.test {
			out = io.Discard
		}
		return errors.Join(runGzipStream(ctx, source, out, opts), source.Close())
	}

	target := operand + ".gz"
	if opts.decompress {
		if !strings.HasSuffix(operand, ".gz") {
			return fmt.Errorf("unknown suffix -- ignored")
		}
		target = strings.TrimSuffix(operand, ".gz")
	}
	targetPath := e.Resolve(target)
	out, err := e.FS.Create(targetPath)
	if err != nil {
		_ = source.Close()
		return err
	}
	runErr := runGzipStream(ctx, source, out, opts)
	closeErr := errors.Join(source.Close(), out.Close())
	if runErr != nil || closeErr != nil {
		_ = e.FS.Remove(targetPath)
		return errors.Join(runErr, closeErr)
	}
	if !opts.keep {
		if err := e.FS.Remove(e.Resolve(operand)); err != nil {
			return err
		}
	}
	return nil
}

func runGzipStream(ctx context.Context, input io.Reader, output io.Writer, opts gzipOptions) error {
	if opts.decompress || opts.test {
		reader, err := gzip.NewReader(input)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, &contextReader{ctx: ctx, reader: reader})
		closeErr := reader.Close()
		return errors.Join(copyErr, closeErr, ctx.Err())
	}
	writer, err := gzip.NewWriterLevel(output, opts.level)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(writer, &contextReader{ctx: ctx, reader: input})
	closeErr := writer.Close()
	return errors.Join(copyErr, closeErr, ctx.Err())
}

func parseGzipArgs(e *Env) (gzipOptions, bool) {
	opts := gzipOptions{level: gzip.DefaultCompression}
	args := e.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			opts.operands = append(opts.operands, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			opts.operands = append(opts.operands, arg)
			continue
		}
		switch arg {
		case "--stdout", "--to-stdout":
			opts.stdout = true
			continue
		case "--decompress", "--uncompress":
			opts.decompress = true
			continue
		case "--keep":
			opts.keep = true
			continue
		case "--test":
			opts.test = true
			continue
		case "--force", "--no-name", "--name", "--quiet":
			continue
		}
		for _, flag := range strings.TrimPrefix(arg, "-") {
			switch {
			case flag == 'c':
				opts.stdout = true
			case flag == 'd':
				opts.decompress = true
			case flag == 'k':
				opts.keep = true
			case flag == 't':
				opts.test = true
			case flag == 'f' || flag == 'n' || flag == 'N' || flag == 'q':
			case flag >= '1' && flag <= '9':
				opts.level = int(flag - '0')
			default:
				e.Errorf("unsupported option -- %c", flag)
				return gzipOptions{}, false
			}
		}
	}
	return opts, true
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
