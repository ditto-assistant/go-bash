package gobash

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

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
	return s.fs.OpenFile(resolve(hc.Dir, p), flag, perm)
}

// statHandler backs path tests (e.g. "[ -f x ]") with the virtual filesystem.
func (s *Shell) statHandler(_ context.Context, name string, _ bool) (fs.FileInfo, error) {
	// mvdan resolves test operands against the runner's current directory before
	// invoking StatHandler, but does not consistently attach HandlerContext.
	// Treating name as the already-resolved virtual path avoids a panic for
	// ordinary expressions such as `[ -f /results/001.json ]`.
	return s.fs.Stat(path.Clean(name))
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
