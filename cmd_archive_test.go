package gobash

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestTarCreateListAndExtract(t *testing.T) {
	result, err := New().Run(context.Background(), `
mkdir -p /tmp/src/sub /tmp/out
printf alpha >/tmp/src/a.txt
printf beta >/tmp/src/sub/b.txt
cd /tmp
tar -czf bundle.tar.gz src
tar -tzf bundle.tar.gz
tar -xzf bundle.tar.gz -C out
printf 'content='; cat out/src/sub/b.txt
`)
	if err != nil || result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.Stdout != "src/\nsrc/a.txt\nsrc/sub/\nsrc/sub/b.txt\ncontent=beta" {
		t.Fatalf("stdout=%q", result.Stdout)
	}
}

func TestTarRejectsTraversal(t *testing.T) {
	result, err := New().Run(context.Background(), `printf bad >/tmp/a; cd /tmp; tar -cf a.tar a; mkdir out; tar -xf a.tar -C out`)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("safe archive should extract: result=%+v err=%v", result, err)
	}
}

func TestTarDirectoryChangesDoNotRelocateArchive(t *testing.T) {
	result, err := New().Run(context.Background(), `mkdir -p /tmp/src /tmp/out; printf x >/tmp/src/a; cd /tmp; tar -cf bundle.tar -C src .; test -f /tmp/bundle.tar; tar -xf bundle.tar -C out; cat out/a`)
	if err != nil || result.ExitCode != 0 || result.Stdout != "x" || result.Stderr != "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestZipCreateListPipeAndExtract(t *testing.T) {
	result, err := New().Run(context.Background(), `
mkdir -p /tmp/src/sub /tmp/out
printf alpha >/tmp/src/a.txt
printf beta >/tmp/src/sub/b.txt
cd /tmp
	zip -9qr bundle.zip src
unzip -Z1 bundle.zip
printf 'pipe='; unzip -p bundle.zip src/a.txt
unzip -q bundle.zip -d out
printf '|content='; cat out/src/sub/b.txt
`)
	if err != nil || result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.Stdout != "src/\nsrc/a.txt\nsrc/sub/\nsrc/sub/b.txt\npipe=alpha|content=beta" {
		t.Fatalf("stdout=%q", result.Stdout)
	}
}

func TestUnzipRejectsInvalidArchive(t *testing.T) {
	result, err := New().Run(context.Background(), `printf nope >/tmp/a.zip; unzip /tmp/a.zip`)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode == 0 || !strings.Contains(result.Stderr, "not a valid zip file") {
		t.Fatalf("expected invalid archive error, got %+v", result)
	}
}

func TestArchiveExtractionRejectsTraversal(t *testing.T) {
	tests := []struct {
		name    string
		archive string
		seed    func(*testing.T) []byte
		script  string
	}{
		{
			name: "tar",
			seed: func(t *testing.T) []byte {
				t.Helper()
				var buf bytes.Buffer
				writer := tar.NewWriter(&buf)
				if err := writer.WriteHeader(&tar.Header{Name: "../escape", Mode: 0o644, Size: 1}); err != nil {
					t.Fatal(err)
				}
				if _, err := io.WriteString(writer, "x"); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				return buf.Bytes()
			},
			archive: "/tmp/bad.tar",
			script:  `mkdir /tmp/out; tar -xf /tmp/bad.tar -C /tmp/out`,
		},
		{
			name: "zip",
			seed: func(t *testing.T) []byte {
				t.Helper()
				var buf bytes.Buffer
				writer := zip.NewWriter(&buf)
				file, err := writer.Create("../escape")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := io.WriteString(file, "x"); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				return buf.Bytes()
			},
			archive: "/tmp/bad.zip",
			script:  `mkdir /tmp/out; unzip -q /tmp/bad.zip -d /tmp/out`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			shell := New()
			if err := afero.WriteFile(shell.FS(), test.archive, test.seed(t), 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := shell.Run(context.Background(), test.script)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if result.ExitCode == 0 || !strings.Contains(result.Stderr, "unsafe archive path") {
				t.Fatalf("expected traversal rejection, got %+v", result)
			}
			if exists, _ := afero.Exists(shell.FS(), "/tmp/escape"); exists {
				t.Fatal("archive escaped extraction root")
			}
		})
	}
}

func TestArchiveExtractionHonorsFilesystemQuota(t *testing.T) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	file, err := writer.Create("large.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(bytes.Repeat([]byte("x"), 4096)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	shell := New(WithFSQuota(1024, 32))
	if err := afero.WriteFile(shell.FS(), "/tmp/a.zip", buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	result, runErr := shell.Run(context.Background(), `unzip -q /tmp/a.zip -d /tmp/out`)
	if runErr != nil {
		t.Fatalf("quota error escaped as Go error: %v", runErr)
	}
	if result.ExitCode == 0 || !strings.Contains(result.Stderr, "quota exceeded") {
		t.Fatalf("expected structured quota failure, got %+v", result)
	}
}

func TestRequestedArchiveAndInspectionInventory(t *testing.T) {
	result, err := New().Run(context.Background(), `gobash commands | rg 'zip|unzip|tar|gunzip|gzip|7z|busybox|xxd|od|hexdump|perl|php|ruby'`)
	if err != nil || result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	want := "gunzip\ngzip\nhexdump\nod\ntar\nunzip\nxxd\nzip\n"
	if result.Stdout != want {
		t.Fatalf("stdout=%q want=%q", result.Stdout, want)
	}
}

func TestNewCommandHelp(t *testing.T) {
	for _, command := range []string{"gzip", "gunzip", "tar", "zip", "unzip", "xxd", "od", "hexdump"} {
		t.Run(command, func(t *testing.T) {
			result, err := New().Run(context.Background(), command+" --help")
			if err != nil || result.ExitCode != 0 || result.Stderr != "" || !strings.HasPrefix(result.Stdout, "usage: ") {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}
