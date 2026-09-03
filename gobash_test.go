package gobash

import (
	"context"
	"strings"
	"testing"
)

// run executes a script and fails the test on interpreter errors.
func run(t *testing.T, sh *Shell, script string) Result {
	t.Helper()
	res, err := sh.Run(context.Background(), script)
	if err != nil {
		t.Fatalf("Run(%q) interpreter error: %v", script, err)
	}
	return res
}

func TestEcho(t *testing.T) {
	cases := []struct {
		script string
		want   string
	}{
		{`echo hello world`, "hello world\n"},
		{`echo -n hi`, "hi"},
		{`echo`, "\n"},
	}
	for _, tc := range cases {
		res := run(t, New(), tc.script)
		if res.Stdout != tc.want {
			t.Errorf("echo %q: got %q want %q", tc.script, res.Stdout, tc.want)
		}
	}
}

func TestPipeline(t *testing.T) {
	res := run(t, New(), `echo hello | cat`)
	if res.Stdout != "hello\n" {
		t.Errorf("pipe: got %q", res.Stdout)
	}
}

func TestRedirectAndRead(t *testing.T) {
	sh := New()
	run(t, sh, `mkdir -p /work && echo hi > /work/a.txt`)
	res := run(t, sh, `cat /work/a.txt`)
	if res.Stdout != "hi\n" {
		t.Errorf("redirect/read: got %q", res.Stdout)
	}
}

func TestLsSorted(t *testing.T) {
	sh := New()
	run(t, sh, `mkdir -p /d && touch /d/b.txt /d/a.txt /d/.hidden`)
	res := run(t, sh, `ls /d`)
	if res.Stdout != "a.txt\nb.txt\n" {
		t.Errorf("ls: got %q", res.Stdout)
	}
	res = run(t, sh, `ls -a /d`)
	if res.Stdout != ".hidden\na.txt\nb.txt\n" {
		t.Errorf("ls -a: got %q", res.Stdout)
	}
}

func TestRm(t *testing.T) {
	sh := New()
	run(t, sh, `mkdir -p /d/sub && touch /d/f.txt`)
	run(t, sh, `rm /d/f.txt`)
	res, _ := sh.Run(context.Background(), `cat /d/f.txt`)
	if res.ExitCode == 0 {
		t.Errorf("expected cat of removed file to fail")
	}
	// rm without -r on a directory should fail.
	res = run(t, sh, `rm /d/sub`)
	if res.ExitCode == 0 {
		t.Errorf("rm dir without -r should fail")
	}
	res = run(t, sh, `rm -r /d/sub`)
	if res.ExitCode != 0 {
		t.Errorf("rm -r dir should succeed, got %d / %q", res.ExitCode, res.Stderr)
	}
}

func TestExitCodes(t *testing.T) {
	sh := New()
	res, _ := sh.Run(context.Background(), `nonexistent_cmd_xyz`)
	if res.ExitCode != 127 {
		t.Errorf("command not found: got exit %d", res.ExitCode)
	}
	res = run(t, sh, `false`)
	_ = res
}

func TestAndChaining(t *testing.T) {
	sh := New()
	res := run(t, sh, `mkdir /x && echo ok`)
	if res.Stdout != "ok\n" {
		t.Errorf("&& chain: got %q (stderr %q)", res.Stdout, res.Stderr)
	}
}

func TestIsolationNoHostLeak(t *testing.T) {
	// /etc/passwd exists on the host but must not be visible in the virtual FS.
	res, _ := New().Run(context.Background(), `cat /etc/passwd`)
	if res.ExitCode == 0 {
		t.Errorf("host file leaked into virtual FS: %q", res.Stdout)
	}
}

func TestVirtualFilesystemQuota(t *testing.T) {
	sh := New(WithFSQuota(8, 2))
	res := run(t, sh, `printf 12345678 >/one; cp /one /two`)
	if res.ExitCode == 0 || !strings.Contains(res.Stderr, "quota exceeded") {
		t.Fatalf("expected byte quota failure, got %+v", res)
	}
	_, err := sh.Run(context.Background(), `printf x >/two; printf y >/three`)
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("expected file quota failure, got %v", err)
	}
}

func TestPathTestsUseVirtualFilesystem(t *testing.T) {
	sh := New()
	res := run(t, sh, `mkdir -p /d; touch /d/a; [ -f /d/a ] && [ -d /d ] && printf ok`)
	if res.ExitCode != 0 || res.Stdout != "ok" {
		t.Fatalf("virtual path tests failed: %+v", res)
	}
}
