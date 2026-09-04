package gobash

import "testing"

func TestJqExitStatus(t *testing.T) {
	tests := []struct {
		name     string
		script   string
		wantCode int
		wantOut  string
	}{
		{name: "truthy", script: `printf '%s' '{"ok":true}' | jq -e '.ok'`, wantCode: 0, wantOut: "true\n"},
		{name: "false", script: `printf '%s' '{"ok":false}' | jq -e '.ok'`, wantCode: 1, wantOut: "false\n"},
		{name: "missing is null", script: `printf '%s' '{}' | jq -e '.missing'`, wantCode: 1, wantOut: "null\n"},
		{name: "no output", script: `printf '%s' '{}' | jq -e 'empty'`, wantCode: 4},
		{name: "long option", script: `printf '%s' '{"ok":true}' | jq --exit-status '.ok'`, wantCode: 0, wantOut: "true\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := run(t, New(), test.script)
			if result.ExitCode != test.wantCode || result.Stdout != test.wantOut || result.Stderr != "" {
				t.Fatalf("result=%+v want_code=%d want_stdout=%q", result, test.wantCode, test.wantOut)
			}
		})
	}
}
