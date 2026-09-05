package gobash

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

func init() { Register("tar", cmdTar) }

type tarOptions struct {
	mode      byte
	archive   string
	directory string
	gzip      bool
	verbose   bool
	operands  []string
}

// cmdTar supports GNU-style create, extract, and list workflows with -f, -z,
// -C, -v, combined flags, stdin/stdout archives, and safe VFS extraction.
func cmdTar(ctx context.Context, e *Env) int {
	if len(e.Args) == 2 && e.Args[1] == "--help" {
		_, _ = fmt.Fprintln(e.Stdout, "usage: tar -c|-x|-t [-zv] -f ARCHIVE [-C DIR] [file ...]")
		return 0
	}
	opts, ok := parseTarArgs(e)
	if !ok {
		return 2
	}
	if opts.directory == "" {
		opts.directory = e.Dir
	} else {
		opts.directory = e.Resolve(opts.directory)
	}
	var err error
	switch opts.mode {
	case 'c':
		err = createTar(ctx, e, opts)
	case 't', 'x':
		err = readTar(ctx, e, opts)
	}
	if err != nil {
		e.Errorf("%v", err)
		return 1
	}
	return 0
}

func parseTarArgs(e *Env) (tarOptions, bool) {
	var opts tarOptions
	args := e.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			opts.operands = append(opts.operands, args[i+1:]...)
			break
		}
		if arg == "--gzip" {
			opts.gzip = true
			continue
		}
		if arg == "--verbose" {
			opts.verbose = true
			continue
		}
		if arg == "--file" || arg == "--directory" {
			if i+1 >= len(args) {
				e.Errorf("option %s requires an argument", arg)
				return tarOptions{}, false
			}
			i++
			if arg == "--file" {
				opts.archive = args[i]
			} else {
				opts.directory = args[i]
			}
			continue
		}
		if strings.HasPrefix(arg, "--file=") {
			opts.archive = strings.TrimPrefix(arg, "--file=")
			continue
		}
		if strings.HasPrefix(arg, "--directory=") {
			opts.directory = strings.TrimPrefix(arg, "--directory=")
			continue
		}
		flags := arg
		if strings.HasPrefix(flags, "-") {
			flags = strings.TrimPrefix(flags, "-")
		} else if i != 0 || !strings.ContainsAny(flags, "cxt") {
			opts.operands = append(opts.operands, arg)
			continue
		}
		for j := 0; j < len(flags); j++ {
			switch flags[j] {
			case 'c', 'x', 't':
				if opts.mode != 0 && opts.mode != flags[j] {
					e.Errorf("exactly one operation is required")
					return tarOptions{}, false
				}
				opts.mode = flags[j]
			case 'z':
				opts.gzip = true
			case 'v':
				opts.verbose = true
			case 'f', 'C':
				value := flags[j+1:]
				if value == "" {
					if i+1 >= len(args) {
						e.Errorf("option -%c requires an argument", flags[j])
						return tarOptions{}, false
					}
					i++
					value = args[i]
				}
				if flags[j] == 'f' {
					opts.archive = value
				} else {
					opts.directory = value
				}
				j = len(flags)
			default:
				e.Errorf("unsupported option -- %c", flags[j])
				return tarOptions{}, false
			}
		}
	}
	if opts.mode == 0 || opts.archive == "" {
		e.Errorf("an operation and -f ARCHIVE are required")
		return tarOptions{}, false
	}
	if opts.mode == 'c' && len(opts.operands) == 0 {
		e.Errorf("refusing to create an empty archive")
		return tarOptions{}, false
	}
	return opts, true
}

func createTar(ctx context.Context, e *Env, opts tarOptions) error {
	originalDir := e.Dir
	e.Dir = opts.directory
	entries, err := collectArchiveEntries(e, opts.operands, true)
	e.Dir = originalDir
	if err != nil {
		return err
	}
	output := e.Stdout
	var archiveFile io.Closer
	if opts.archive != "-" {
		file, err := e.FS.Create(e.Resolve(opts.archive))
		if err != nil {
			return err
		}
		output, archiveFile = file, file
	}
	if archiveFile != nil {
		defer func() { _ = archiveFile.Close() }()
	}
	var gzipWriter *gzip.Writer
	if opts.gzip {
		gzipWriter = gzip.NewWriter(output)
		output = gzipWriter
	}
	tw := tar.NewWriter(output)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(entry.info, "")
		if err != nil {
			return err
		}
		header.Name = entry.name
		if entry.info.IsDir() && !strings.HasSuffix(header.Name, "/") {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if entry.info.Mode().IsRegular() {
			file, err := e.FS.Open(entry.source)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(tw, &contextReader{ctx: ctx, reader: file})
			closeErr := file.Close()
			if err := errors.Join(copyErr, closeErr); err != nil {
				return err
			}
		}
		if opts.verbose {
			_, _ = fmt.Fprintln(e.Stderr, header.Name)
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if gzipWriter != nil {
		return gzipWriter.Close()
	}
	return nil
}

func readTar(ctx context.Context, e *Env, opts tarOptions) error {
	input := e.Stdin
	var archiveFile io.Closer
	if opts.archive != "-" {
		file, err := e.FS.Open(e.Resolve(opts.archive))
		if err != nil {
			return err
		}
		input, archiveFile = file, file
	}
	if archiveFile != nil {
		defer func() { _ = archiveFile.Close() }()
	}
	var gzipReader *gzip.Reader
	if opts.gzip {
		var err error
		gzipReader, err = gzip.NewReader(input)
		if err != nil {
			return err
		}
		defer func() { _ = gzipReader.Close() }()
		input = gzipReader
	}
	tr := tar.NewReader(input)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if !matchesArchiveSelection(header.Name, opts.operands) {
			continue
		}
		if opts.mode == 't' {
			_, err = fmt.Fprintln(e.Stdout, header.Name)
			if err != nil {
				return err
			}
			continue
		}
		target, err := safeArchiveTarget(opts.directory, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = e.FS.MkdirAll(target, archiveMode(header.FileInfo().Mode().Perm(), true))
		case tar.TypeReg:
			if err = e.FS.MkdirAll(path.Dir(target), 0o755); err == nil {
				var file io.WriteCloser
				file, err = createArchiveFile(e, target, archiveMode(header.FileInfo().Mode().Perm(), false))
				if err == nil {
					_, copyErr := io.Copy(file, &contextReader{ctx: ctx, reader: tr})
					closeErr := file.Close()
					err = errors.Join(copyErr, closeErr)
				}
			}
		default:
			err = fmt.Errorf("unsupported archive entry type for %q", header.Name)
		}
		if err != nil {
			return err
		}
		if opts.verbose {
			_, _ = fmt.Fprintln(e.Stdout, header.Name)
		}
	}
}

func createArchiveFile(e *Env, target string, mode fs.FileMode) (io.WriteCloser, error) {
	file, err := e.FS.Create(target)
	if err != nil {
		return nil, err
	}
	if err := e.FS.Chmod(target, mode); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func archiveMode(mode fs.FileMode, directory bool) fs.FileMode {
	if mode.Perm() != 0 {
		return mode.Perm()
	}
	if directory {
		return 0o755
	}
	return 0o644
}
