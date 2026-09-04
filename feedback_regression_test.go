package gobash

import (
	"strings"
	"testing"
	"time"
)

func TestFeedbackCommandDiscoveryAndRuntimeIdentity(t *testing.T) {
	commands := []string{
		"bash", "ls", "cat", "head", "tail", "sed", "awk", "grep", "rg",
		"sort", "uniq", "wc", "tr", "cut", "find", "xargs", "seq", "date",
		"env", "printenv", "jq", "mkdir", "cp", "mv", "rm", "mktemp", "pwd",
	}
	script := `for name in ` + strings.Join(commands, " ") + `; do command -v "$name" || exit 10; done
printf 'shell=%s cwd=%s user=%s\n' "$0" "$PWD" "$(whoami)"`
	result := run(t, New(), script)
	if result.ExitCode != 0 {
		t.Fatalf("discovery failed: %+v", result)
	}
	for _, command := range commands {
		if !strings.Contains(result.Stdout, command+"\n") {
			t.Fatalf("%s not discoverable in %q", command, result.Stdout)
		}
	}
	if !strings.HasSuffix(result.Stdout, "shell=bash cwd=/tmp user=agent\n") {
		t.Fatalf("identity mismatch: %q", result.Stdout)
	}
	missing := run(t, New(), `command -v zsh`)
	if missing.ExitCode != 1 || missing.Stdout != "" {
		t.Fatalf("unsupported zsh should remain unavailable: %+v", missing)
	}
}

func TestResultMetadataStreamsAndTimeout(t *testing.T) {
	result := run(t, New(WithTimeout(20*time.Millisecond)), `printf stdout; printf stderr >&2`)
	if result.Stdout != "stdout" || result.Stderr != "stderr" {
		t.Fatalf("streams were not separated: %+v", result)
	}
	if result.Shell != "bash" || result.Runtime != "go-bash/mvdan-sh" || result.Cwd != "/tmp" || result.TimeoutMS != 20 {
		t.Fatalf("metadata mismatch: %+v", result)
	}
	timedOut := run(t, New(WithTimeout(time.Millisecond)), `while true; do :; done`)
	if timedOut.ExitCode != 124 || !strings.Contains(timedOut.Stderr, "timed out") {
		t.Fatalf("timeout mismatch: %+v", timedOut)
	}
}
