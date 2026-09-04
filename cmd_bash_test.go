package gobash

import (
	"strings"
	"testing"
)

func TestBashHelpAndNestedShellBoundary(t *testing.T) {
	result := run(t, New(), `bash --help`)
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "Bash-compatible") {
		t.Fatalf("bash help: %+v", result)
	}
	result = run(t, New(), `bash -c 'echo unsafe'`)
	if result.ExitCode != 2 || !strings.Contains(result.Stderr, "nested shells are unavailable") {
		t.Fatalf("nested bash: %+v", result)
	}
}
