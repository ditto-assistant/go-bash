package gobash

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

func init() {
	Register("zip", cmdZip)
	Register("unzip", cmdUnzip)
}

type zipOptions struct {
	recursive bool
	quiet     bool
	store     bool
	level     int
	archive   string
	operands  []string
}

// cmdZip creates VFS-local ZIP archives. It supports -r, -q, -0, stdin/stdout
// archives through "-", and the ordinary `zip archive files...` form.
func cmdZip(ctx context.Context, e *Env) int {
	if len(e.Args) == 2 && e.Args[1] == "--help" {
		_, _ = fmt.Fprintln(e.Stdout, "usage: zip [-qr0] ARCHIVE FILE...")
		return 0
	}
	opts, ok := parseZipArgs(e)
	if !ok {
		return 2
	}
	entries, err := collectArchiveEntries(e, opts.operands, opts.recursive)
	if err != nil {
		e.Errorf("%v", err)
		return 1
	}
	var output io.Writer = e.Stdout
	var archiveFile io.Closer
	if opts.archive != "-" {
		file, err := e.FS.Create(e.Resolve(opts.archive))
		if err != nil {
			e.Errorf("%s: %v", opts.archive, err)
			return 1
		}
		output, archiveFile = file, file
	}
	zw := zip.NewWriter(output)
	if !opts.store {
		zw.RegisterCompressor(zip.Deflate, func(writer io.Writer) (io.WriteCloser, error) {
			return flate.NewWriter(writer, opts.level)
		})
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			e.Errorf("%v", err)
			_ = zw.Close()
			if archiveFile != nil {
				_ = archiveFile.Close()
			}
			return 1
		}
		header, err := zip.FileInfoHeader(entry.info)
		if err != nil {
			e.Errorf("%v", err)
			return 1
		}
		header.Name = entry.name
		if entry.info.IsDir() {
			header.Name = strings.TrimSuffix(header.Name, "/") + "/"
		} else if !opts.store {
			header.Method = zip.Deflate
		}
		writer, err := zw.CreateHeader(header)
		if err != nil {
			e.Errorf("%v", err)
			return 1
		}
		if entry.info.Mode().IsRegular() {
			file, err := e.FS.Open(entry.source)
			if err != nil {
				e.Errorf("%v", err)
				return 1
			}
			_, copyErr := io.Copy(writer, &contextReader{ctx: ctx, reader: file})
			closeErr := file.Close()
			if err := errors.Join(copyErr, closeErr); err != nil {
				e.Errorf("%v", err)
				return 1
			}
		}
		if !opts.quiet && opts.archive != "-" {
			_, _ = fmt.Fprintf(e.Stdout, "  adding: %s\n", header.Name)
		}
	}
	closeErr := zw.Close()
	if archiveFile != nil {
		closeErr = errors.Join(closeErr, archiveFile.Close())
	}
	if closeErr != nil {
		e.Errorf("%v", closeErr)
		return 1
	}
	return 0
}

func parseZipArgs(e *Env) (zipOptions, bool) {
	opts := zipOptions{level: flate.DefaultCompression}
	args := e.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			opts.operands = append(opts.operands, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" && opts.archive == "" {
			for _, flag := range strings.TrimPrefix(arg, "-") {
				switch flag {
				case 'r':
					opts.recursive = true
				case 'q':
					opts.quiet = true
				case '0':
					opts.store = true
				case '1', '2', '3', '4', '5', '6', '7', '8', '9':
					opts.level = int(flag - '0')
				default:
					e.Errorf("unsupported option -- %c", flag)
					return zipOptions{}, false
				}
			}
			continue
		}
		if opts.archive == "" {
			opts.archive = arg
		} else {
			opts.operands = append(opts.operands, arg)
		}
	}
	if opts.archive == "" || len(opts.operands) == 0 {
		e.Errorf("archive and at least one input are required")
		return zipOptions{}, false
	}
	return opts, true
}

type unzipOptions struct {
	list       bool
	namesOnly  bool
	pipe       bool
	quiet      bool
	neverWrite bool
	directory  string
	archive    string
	selections []string
}

// cmdUnzip supports -l, -Z1, -p, -q, -d, -o, and -n. Extraction rejects
// absolute paths and traversal before touching the virtual filesystem.
func cmdUnzip(ctx context.Context, e *Env) int {
	if len(e.Args) == 2 && e.Args[1] == "--help" {
		_, _ = fmt.Fprintln(e.Stdout, "usage: unzip [-lZ1pqon] ARCHIVE [file ...] [-d DIR]")
		return 0
	}
	opts, ok := parseUnzipArgs(e)
	if !ok {
		return 2
	}
	data, err := readVirtualOrStdin(e, opts.archive)
	if err != nil {
		e.Errorf("%s: %v", opts.archive, err)
		return 1
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		e.Errorf("%s: not a valid zip file: %v", opts.archive, err)
		return 1
	}
	root := e.Dir
	if opts.directory != "" {
		root = e.Resolve(opts.directory)
	}
	for _, member := range zr.File {
		if err := ctx.Err(); err != nil {
			e.Errorf("%v", err)
			return 1
		}
		if !matchesArchiveSelection(member.Name, opts.selections) {
			continue
		}
		if !member.FileInfo().IsDir() && !member.Mode().IsRegular() {
			e.Errorf("%s: unsupported special file", member.Name)
			return 1
		}
		if opts.namesOnly {
			_, _ = fmt.Fprintln(e.Stdout, member.Name)
			continue
		}
		if opts.list {
			_, _ = fmt.Fprintf(e.Stdout, "%9d  %s\n", member.UncompressedSize64, member.Name)
			continue
		}
		reader, err := member.Open()
		if err != nil {
			e.Errorf("%s: %v", member.Name, err)
			return 1
		}
		if opts.pipe {
			_, err = io.Copy(e.Stdout, &contextReader{ctx: ctx, reader: reader})
			err = errors.Join(err, reader.Close())
			if err != nil {
				e.Errorf("%s: %v", member.Name, err)
				return 1
			}
			continue
		}
		target, err := safeArchiveTarget(root, member.Name)
		if err != nil {
			_ = reader.Close()
			e.Errorf("%v", err)
			return 1
		}
		if member.FileInfo().IsDir() {
			_ = reader.Close()
			if err := e.FS.MkdirAll(target, archiveMode(member.Mode().Perm(), true)); err != nil {
				e.Errorf("%s: %v", member.Name, err)
				return 1
			}
			continue
		}
		if exists, _ := aferoExists(e, target); exists && opts.neverWrite {
			_ = reader.Close()
			continue
		}
		if err := e.FS.MkdirAll(path.Dir(target), 0o755); err != nil {
			_ = reader.Close()
			e.Errorf("%s: %v", member.Name, err)
			return 1
		}
		file, err := createArchiveFile(e, target, archiveMode(member.Mode().Perm(), false))
		if err != nil {
			_ = reader.Close()
			e.Errorf("%s: %v", member.Name, err)
			return 1
		}
		_, copyErr := io.Copy(file, &contextReader{ctx: ctx, reader: reader})
		if err := errors.Join(copyErr, file.Close(), reader.Close()); err != nil {
			e.Errorf("%s: %v", member.Name, err)
			return 1
		}
		if !opts.quiet {
			_, _ = fmt.Fprintf(e.Stdout, "  inflating: %s\n", member.Name)
		}
	}
	return 0
}

func parseUnzipArgs(e *Env) (unzipOptions, bool) {
	var opts unzipOptions
	args := e.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-d" {
			if i+1 >= len(args) {
				e.Errorf("option -d requires an argument")
				return unzipOptions{}, false
			}
			i++
			opts.directory = args[i]
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" && opts.archive == "" {
			flags := strings.TrimPrefix(arg, "-")
			for i := 0; i < len(flags); i++ {
				switch flags[i] {
				case 'l':
					opts.list = true
				case 'Z':
					opts.namesOnly = true
				case '1':
					if !opts.namesOnly {
						e.Errorf("-1 is only supported with -Z")
						return unzipOptions{}, false
					}
				case 'p':
					opts.pipe = true
				case 'q':
					opts.quiet = true
				case 'o':
				case 'n':
					opts.neverWrite = true
				default:
					e.Errorf("unsupported option -- %c", flags[i])
					return unzipOptions{}, false
				}
			}
			continue
		}
		if opts.archive == "" {
			opts.archive = arg
		} else {
			opts.selections = append(opts.selections, arg)
		}
	}
	if opts.archive == "" {
		e.Errorf("archive is required")
		return unzipOptions{}, false
	}
	return opts, true
}

func readVirtualOrStdin(e *Env, name string) ([]byte, error) {
	if name == "-" {
		return io.ReadAll(e.Stdin)
	}
	return aferoReadFile(e, e.Resolve(name))
}

func aferoReadFile(e *Env, name string) ([]byte, error) {
	file, err := e.FS.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func aferoExists(e *Env, name string) (bool, error) {
	_, err := e.FS.Stat(name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}
