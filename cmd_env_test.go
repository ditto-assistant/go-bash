package gobash

import (
	"strings"
	"testing"
)

func TestEnvListsOnlySandboxEnvironment(t *testing.T) {
	result := run(t, New(WithEnv("CUSTOM", "value")), `env`)
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "CUSTOM=value\n") || !strings.Contains(result.Stdout, "GOBASH_RUNTIME=mvdan-sh\n") {
		t.Fatalf("env: %+v", result)
	}
	if strings.Contains(result.Stdout, "SSH_AUTH_SOCK=") {
		t.Fatalf("host environment leaked: %q", result.Stdout)
	}
}
