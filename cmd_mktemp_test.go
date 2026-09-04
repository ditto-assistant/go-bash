package gobash

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestMktempCreatesVirtualPaths(t *testing.T) {
	sh := New()
	fileResult := run(t, sh, `mktemp`)
	file := strings.TrimSpace(fileResult.Stdout)
	if fileResult.ExitCode != 0 || !strings.HasPrefix(file, "/tmp/tmp.") {
		t.Fatalf("mktemp file: %+v", fileResult)
	}
	if exists, _ := afero.Exists(sh.FS(), file); !exists {
		t.Fatalf("temporary file %q not in VFS", file)
	}
	dirResult := run(t, sh, `mktemp -d work.XXXX`)
	dir := strings.TrimSpace(dirResult.Stdout)
	if dirResult.ExitCode != 0 || !strings.HasPrefix(dir, "/tmp/work.") {
		t.Fatalf("mktemp dir: %+v", dirResult)
	}
	if isDir, _ := afero.IsDir(sh.FS(), dir); !isDir {
		t.Fatalf("temporary directory %q not in VFS", dir)
	}
}
