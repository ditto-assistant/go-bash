package gobash

import "testing"

func TestWhoami(t *testing.T) {
	result := run(t, New(), `whoami`)
	if result.ExitCode != 0 || result.Stdout != "agent\n" {
		t.Fatalf("whoami: %+v", result)
	}
}
