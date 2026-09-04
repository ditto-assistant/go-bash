package gobash

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"time"

	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// virtualDeviceFS exposes shell devices to both redirections and the
// file-oriented Go utilities. Embedding keeps all ordinary paths delegated to
// the quota-protected VFS.
type virtualDeviceFS struct{ afero.Fs }

func newVirtualDeviceFS(base afero.Fs) afero.Fs { return &virtualDeviceFS{Fs: base} }

func (v *virtualDeviceFS) Create(name string) (afero.File, error) {
	if path.Clean(name) == "/dev/null" {
		return virtualNullFile{}, nil
	}
	return v.Fs.Create(name)
}

func (v *virtualDeviceFS) Open(name string) (afero.File, error) {
	if path.Clean(name) == "/dev/null" {
		return virtualNullFile{}, nil
	}
	return v.Fs.Open(name)
}

func (v *virtualDeviceFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if path.Clean(name) == "/dev/null" {
		return virtualNullFile{}, nil
	}
	return v.Fs.OpenFile(name, flag, perm)
}

func (v *virtualDeviceFS) Stat(name string) (fs.FileInfo, error) {
	if path.Clean(name) == "/dev/null" {
		return virtualNullInfo{}, nil
	}
	return v.Fs.Stat(name)
}

func (v *virtualDeviceFS) Remove(name string) error {
	if path.Clean(name) == "/dev/null" {
		return fs.ErrPermission
	}
	return v.Fs.Remove(name)
}

func (v *virtualDeviceFS) Rename(oldname, newname string) error {
	if path.Clean(oldname) == "/dev/null" || path.Clean(newname) == "/dev/null" {
		return fs.ErrPermission
	}
	return v.Fs.Rename(oldname, newname)
}

// resolve cleans p into an absolute virtual path relative to dir.
func resolve(dir, p string) string {
	if path.IsAbs(p) {
		return path.Clean(p)
	}
	return path.Clean(path.Join(dir, p))
}

// openHandler backs shell redirects (e.g. "> file", "< file") with the virtual
// filesystem so no host file is ever opened.
func (s *Shell) openHandler(ctx context.Context, p string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
	hc := interp.HandlerCtx(ctx)
	name := resolve(hc.Dir, p)
	if name == "/dev/null" {
		return virtualNull{}, nil
	}
	if flag&os.O_CREATE != 0 {
		parent := path.Dir(name)
		info, err := s.fs.Stat(parent)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s: not a directory", parent)
		}
	}
	f, err := s.fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	state, _ := ctx.Value(runStateKey{}).(*runState)
	return &writeErrorFile{ReadWriteCloser: f, state: state}, nil
}

// statHandler backs path tests (e.g. "[ -f x ]") with the virtual filesystem.
func (s *Shell) statHandler(_ context.Context, name string, _ bool) (fs.FileInfo, error) {
	// mvdan resolves test operands against the runner's current directory before
	// invoking StatHandler, but does not consistently attach HandlerContext.
	// Treating name as the already-resolved virtual path avoids a panic for
	// ordinary expressions such as `[ -f /results/001.json ]`.
	name = path.Clean(name)
	if name == "/dev/null" {
		return virtualNullInfo{}, nil
	}
	return s.fs.Stat(name)
}

// readDirHandler backs globbing with the virtual filesystem.
func (s *Shell) readDirHandler(ctx context.Context, p string) ([]fs.DirEntry, error) {
	hc := interp.HandlerCtx(ctx)
	infos, err := afero.ReadDir(s.fs, resolve(hc.Dir, p))
	if err != nil {
		return nil, err
	}
	entries := make([]fs.DirEntry, len(infos))
	for i, fi := range infos {
		entries[i] = fs.FileInfoToDirEntry(fi)
	}
	return entries, nil
}

// accessHandler keeps cd and -r/-w/-x tests inside the virtual filesystem.
// mvdan/sh v3.14 exposes this hook specifically for non-host filesystems.
func (s *Shell) accessHandler(_ context.Context, name string, mode interp.AccessMode) error {
	if path.Clean(name) == "/dev/null" {
		if mode&interp.AccessExec != 0 {
			return fs.ErrPermission
		}
		return nil
	}
	info, err := s.fs.Stat(path.Clean(name))
	if err != nil {
		return err
	}
	perm := info.Mode().Perm()
	if mode&interp.AccessRead != 0 && perm&0o444 == 0 ||
		mode&interp.AccessWrite != 0 && perm&0o222 == 0 ||
		mode&interp.AccessExec != 0 && perm&0o111 == 0 {
		return fs.ErrPermission
	}
	return nil
}

type virtualNull struct{}

func (virtualNull) Read([]byte) (int, error)    { return 0, io.EOF }
func (virtualNull) Write(p []byte) (int, error) { return len(p), nil }
func (virtualNull) Close() error                { return nil }

// virtualNullFile is the afero.File form used by external commands. Reads are
// always EOF and writes always succeed without storing bytes.
type virtualNullFile struct{ virtualNull }

func (virtualNullFile) Name() string                           { return "/dev/null" }
func (virtualNullFile) ReadAt([]byte, int64) (int, error)      { return 0, io.EOF }
func (virtualNullFile) Seek(int64, int) (int64, error)         { return 0, nil }
func (virtualNullFile) WriteAt(p []byte, _ int64) (int, error) { return len(p), nil }
func (virtualNullFile) Readdir(int) ([]os.FileInfo, error)     { return nil, fs.ErrInvalid }
func (virtualNullFile) Readdirnames(int) ([]string, error)     { return nil, fs.ErrInvalid }
func (virtualNullFile) Stat() (os.FileInfo, error)             { return virtualNullInfo{}, nil }
func (virtualNullFile) Sync() error                            { return nil }
func (virtualNullFile) Truncate(int64) error                   { return nil }
func (virtualNullFile) WriteString(value string) (int, error)  { return len(value), nil }

type virtualNullInfo struct{}

func (virtualNullInfo) Name() string       { return "null" }
func (virtualNullInfo) Size() int64        { return 0 }
func (virtualNullInfo) Mode() fs.FileMode  { return os.ModeDevice | 0o666 }
func (virtualNullInfo) ModTime() time.Time { return time.Time{} }
func (virtualNullInfo) IsDir() bool        { return false }
func (virtualNullInfo) Sys() any           { return nil }

type writeErrorFile struct {
	io.ReadWriteCloser
	state *runState
}

func (f *writeErrorFile) Write(p []byte) (int, error) {
	n, err := f.ReadWriteCloser.Write(p)
	if f.state != nil {
		f.state.recordWriteError(err)
	}
	return n, err
}
