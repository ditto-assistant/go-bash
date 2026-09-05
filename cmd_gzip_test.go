package gobash

import (
	"context"
	"strings"
	"testing"
)

func TestGzipRoundTripsStreamsAndFiles(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{name: "pipeline", script: `printf 'hello\n' | gzip -c | gunzip -c`, want: "hello\n"},
		{name: "file", script: `printf payload >/tmp/a; gzip /tmp/a; test ! -e /tmp/a; gunzip /tmp/a.gz; cat /tmp/a`, want: "payload"},
		{name: "keep", script: `printf payload >/tmp/a; gzip -k /tmp/a; test -f /tmp/a; test -f /tmp/a.gz; gunzip -k /tmp/a.gz; cat /tmp/a`, want: "payload"},
		{name: "test", script: `printf payload | gzip -c >/tmp/a.gz; gzip -t /tmp/a.gz; printf valid`, want: "valid"},
		{name: "empty", script: `printf '' | gzip -c | gunzip -c; printf empty-ok`, want: "empty-ok"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := New().Run(context.Background(), test.script)
			if err != nil || result.ExitCode != 0 || result.Stdout != test.want || result.Stderr != "" {
				t.Fatalf("result=%+v err=%v want_stdout=%q", result, err, test.want)
			}
		})
	}
}

func TestGzipReportsInvalidInput(t *testing.T) {
	result, err := New().Run(context.Background(), `printf nope | gunzip -c`)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode == 0 || (!strings.Contains(result.Stderr, "invalid header") && !strings.Contains(result.Stderr, "unexpected EOF")) {
		t.Fatalf("expected decompression failure, got %+v", result)
	}
}
