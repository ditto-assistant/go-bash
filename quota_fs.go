package gobash

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"
)

// quotaFS caps all writes, including shell redirections and command-created
// files. Accounting is serialized with Shell.Run, but the lock also keeps the
// wrapper correct when callers seed or inspect the VFS between runs.
type quotaFS struct {
	afero.Fs
	sizes    map[string]int64
	bytes    int64
	maxBytes int64
	maxFiles int
	mu       sync.Mutex
}

func newQuotaFS(base afero.Fs, maxBytes int64, maxFiles int) afero.Fs {
	q := &quotaFS{Fs: base, sizes: make(map[string]int64), maxBytes: maxBytes, maxFiles: maxFiles}
	q.rescanLocked()
	return q
}

func (q *quotaFS) Name() string { return "quotaFs" }

func (q *quotaFS) Create(name string) (afero.File, error) {
	return q.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

func (q *quotaFS) Open(name string) (afero.File, error) {
	f, err := q.Fs.Open(name)
	if err != nil {
		return nil, err
	}
	return &quotaFile{File: f, owner: q, name: cleanQuotaPath(name)}, nil
}

func (q *quotaFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	name = cleanQuotaPath(name)
	q.mu.Lock()
	defer q.mu.Unlock()
	_, exists := q.sizes[name]
	if !exists && flag&os.O_CREATE != 0 && len(q.sizes) >= q.maxFiles {
		return nil, q.limitError()
	}
	f, err := q.Fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	info, statErr := f.Stat()
	if statErr != nil {
		_ = f.Close()
		return nil, statErr
	}
	if !info.IsDir() {
		oldSize := q.sizes[name]
		q.sizes[name] = info.Size()
		q.bytes += info.Size() - oldSize
		if q.bytes > q.maxBytes {
			_ = f.Close()
			q.rescanLocked()
			return nil, q.limitError()
		}
	}
	return &quotaFile{File: f, owner: q, name: name}, nil
}

func (q *quotaFS) Remove(name string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.Fs.Remove(name); err != nil {
		return err
	}
	q.removeAccountingLocked(cleanQuotaPath(name))
	return nil
}

func (q *quotaFS) RemoveAll(name string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.Fs.RemoveAll(name); err != nil {
		return err
	}
	root := cleanQuotaPath(name)
	for file := range q.sizes {
		if file == root || strings.HasPrefix(file, strings.TrimRight(root, "/")+"/") {
			q.removeAccountingLocked(file)
		}
	}
	return nil
}

func (q *quotaFS) Rename(oldname, newname string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.Fs.Rename(oldname, newname); err != nil {
		return err
	}
	q.rescanLocked()
	return nil
}

func (q *quotaFS) Chown(name string, uid, gid int) error {
	if owner, ok := q.Fs.(interface{ Chown(string, int, int) error }); ok {
		return owner.Chown(name, uid, gid)
	}
	return fmt.Errorf("chown is unsupported")
}

func (q *quotaFS) Chmod(name string, mode os.FileMode) error { return q.Fs.Chmod(name, mode) }

func (q *quotaFS) Chtimes(name string, atime, mtime time.Time) error {
	return q.Fs.Chtimes(name, atime, mtime)
}

func (q *quotaFS) limitError() error {
	return fmt.Errorf("gobash: virtual filesystem quota exceeded (%d bytes, %d files)", q.maxBytes, q.maxFiles)
}

func (q *quotaFS) removeAccountingLocked(name string) {
	if size, ok := q.sizes[name]; ok {
		q.bytes -= size
		delete(q.sizes, name)
	}
}

func (q *quotaFS) rescanLocked() {
	q.sizes = make(map[string]int64)
	q.bytes = 0
	_ = afero.Walk(q.Fs, "/", func(name string, info fs.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			name = cleanQuotaPath(name)
			q.sizes[name] = info.Size()
			q.bytes += info.Size()
		}
		return nil
	})
}

func cleanQuotaPath(name string) string {
	if !path.IsAbs(name) {
		name = "/" + name
	}
	return path.Clean(name)
}

type quotaFile struct {
	afero.File
	owner *quotaFS
	name  string
}

func (f *quotaFile) Write(p []byte) (int, error) {
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	return f.writeAtCurrent(p, offset, func() (int, error) { return f.File.Write(p) })
}

func (f *quotaFile) WriteAt(p []byte, offset int64) (int, error) {
	return f.writeAtCurrent(p, offset, func() (int, error) { return f.File.WriteAt(p, offset) })
}

func (f *quotaFile) WriteString(value string) (int, error) { return f.Write([]byte(value)) }

func (f *quotaFile) Truncate(size int64) error {
	f.owner.mu.Lock()
	defer f.owner.mu.Unlock()
	oldSize := f.owner.sizes[f.name]
	if size > oldSize && f.owner.bytes+size-oldSize > f.owner.maxBytes {
		return f.owner.limitError()
	}
	if err := f.File.Truncate(size); err != nil {
		return err
	}
	f.owner.sizes[f.name] = size
	f.owner.bytes += size - oldSize
	return nil
}

func (f *quotaFile) writeAtCurrent(p []byte, offset int64, write func() (int, error)) (int, error) {
	f.owner.mu.Lock()
	defer f.owner.mu.Unlock()
	oldSize := f.owner.sizes[f.name]
	newSize := max(oldSize, offset+int64(len(p)))
	if f.owner.bytes+newSize-oldSize > f.owner.maxBytes {
		return 0, f.owner.limitError()
	}
	n, err := write()
	actualSize := max(oldSize, offset+int64(n))
	f.owner.sizes[f.name] = actualSize
	f.owner.bytes += actualSize - oldSize
	return n, err
}
