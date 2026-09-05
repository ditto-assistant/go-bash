package gobash

import (
	"context"
	"testing"
)

func TestByteInspectionCommands(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{name: "xxd", script: `printf QUJDAP8= | base64 -d | xxd`, want: "00000000: 4142 4300 ff                             ABC..\n"},
		{name: "xxd plain reverse", script: `printf QUJDAP8= | base64 -d | xxd -p | xxd -r -p | base64 -w0`, want: "QUJDAP8="},
		{name: "od", script: `printf QUJDAP8= | base64 -d | od -An -tx1`, want: " 41 42 43 00 ff\n"},
		{name: "hexdump canonical", script: `printf QUJDAP8= | base64 -d | hexdump -C`, want: "00000000  41 42 43 00 ff                                    |ABC..|\n00000005\n"},
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

func TestByteInspectionEmptyInput(t *testing.T) {
	result, err := New().Run(context.Background(), `printf '' | xxd; printf '' | od -An -tx1; printf '' | hexdump -C`)
	if err != nil || result.ExitCode != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestByteInspectionBounds(t *testing.T) {
	result, err := New().Run(context.Background(), `printf abcdef | xxd -p -s 2 -l 3; printf abcdef | od -An -tx1 -j 2 -N 3; printf abcdef | hexdump -C -s 2 -n 3`)
	if err != nil || result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	want := "636465\n 63 64 65\n00000002  63 64 65                                          |cde|\n00000005\n"
	if result.Stdout != want {
		t.Fatalf("stdout=%q want=%q", result.Stdout, want)
	}
}
