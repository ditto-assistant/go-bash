package gobash

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestDataProcessingToolset(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{name: "grep pipeline", script: `printf 'Alpha\nbeta\nALPINE\n' | grep -in '^al'`, want: "1:Alpha\n3:ALPINE\n"},
		{name: "ripgrep recursive", script: `mkdir -p /d/sub; printf 'no\n' >/d/a; printf 'needle\n' >/d/sub/b; rg -n needle /d`, want: "/d/sub/b:1:needle\n"},
		{name: "head", script: `printf 'a\nb\nc\n' | head -n 2`, want: "a\nb\n"},
		{name: "tail", script: `printf 'a\nb\nc\n' | tail -2`, want: "b\nc\n"},
		{name: "sort uniq", script: `printf 'b\na\na\n' | sort | uniq -c`, want: "      2 a\n      1 b\n"},
		{name: "cut", script: `printf 'a,b,c\n1,2,3\n' | cut -d, -f1,3`, want: "a,c\n1,3\n"},
		{name: "tr", script: `printf 'abc xyz' | tr a-z A-Z`, want: "ABC XYZ"},
		{name: "sed", script: `printf 'old old\nkeep\n' | sed 's/old/new/g'`, want: "new new\nkeep\n"},
		{name: "awk", script: `printf 'a,2\nb,3\n' | awk -F, '$2 > 2 { print $1 }'`, want: "b\n"},
		{name: "jq", script: `printf '%s' '{"items":[{"name":"a","score":1},{"name":"b","score":2}]}' | jq -r '.items[] | select(.score > 1) | .name'`, want: "b\n"},
		{name: "jq compact", script: `printf '%s' '{"a":1}' | jq -c '{value:.a}'`, want: "{\"value\":1}\n"},
		{name: "find", script: `mkdir -p /d/sub; touch /d/a.json /d/sub/b.txt; find /d -type f -name '*.json'`, want: "/d/a.json\n"},
		{name: "copy recursive", script: `mkdir -p /a/sub; echo yes >/a/sub/f; cp -r /a /b; cat /b/sub/f`, want: "yes\n"},
		{name: "base64 round trip", script: `printf hello | base64 | base64 -d`, want: "hello"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := New().Run(context.Background(), tc.script)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if result.ExitCode != 0 {
				t.Fatalf("exit=%d stderr=%q", result.ExitCode, result.Stderr)
			}
			if result.Stdout != tc.want {
				t.Fatalf("stdout=%q want=%q", result.Stdout, tc.want)
			}
		})
	}
}

func TestJqRejectsHostCapabilities(t *testing.T) {
	result, err := New().Run(context.Background(), `echo '{}' | jq 'input_filename'`)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode == 0 || !strings.Contains(result.Stderr, "input_filename") {
		t.Fatalf("expected unavailable input filename to fail closed: %+v", result)
	}
}

func TestRunLimits(t *testing.T) {
	result, err := New(WithLimits(4, 128, 2)).Run(context.Background(), `printf 12345; cat /dev/null; cat /dev/null; cat /dev/null`)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Stdout != "1234" || !result.Truncated {
		t.Fatalf("expected capped output, got %+v", result)
	}
	if result.ExitCode != 124 {
		t.Fatalf("expected command limit, got %+v", result)
	}
}

func TestRunIOLimitsCommandsAndScript(t *testing.T) {
	sh := New(WithLimits(64, 8, 1))
	var stdout, stderr bytes.Buffer
	code, err := sh.RunIO(context.Background(), "cat; cat", strings.NewReader(""), &stdout, &stderr)
	if err != nil || code != 124 || !strings.Contains(stderr.String(), "command limit") {
		t.Fatalf("RunIO command limit: code=%d stdout=%q stderr=%q err=%v", code, stdout.String(), stderr.String(), err)
	}
	code, err = sh.RunIO(context.Background(), "printf toolong", strings.NewReader(""), &stdout, &stderr)
	if err == nil || code != 2 || !strings.Contains(err.Error(), "script exceeds") {
		t.Fatalf("RunIO script limit: code=%d err=%v", code, err)
	}
}

func TestAwkRejectsUnboundedControlFlow(t *testing.T) {
	result, err := New().Run(context.Background(), `awk 'BEGIN { while (1) {} }'`)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode == 0 || !strings.Contains(result.Stderr, "disabled") {
		t.Fatalf("expected bounded awk rejection, got %+v", result)
	}
}
