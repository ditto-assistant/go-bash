package gobash

import "testing"

func TestPrintenv(t *testing.T) {
	result := run(t, New(WithEnv("CUSTOM", "value")), `printenv CUSTOM MISSING`)
	if result.ExitCode != 1 || result.Stdout != "value\n" || result.Stderr != "" {
		t.Fatalf("printenv: %+v", result)
	}
}
