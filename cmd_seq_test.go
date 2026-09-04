package gobash

import "testing"

func TestSeqRanges(t *testing.T) {
	tests := []struct {
		script string
		want   string
	}{
		{`seq 3`, "1\n2\n3\n"},
		{`seq 3 -1 1`, "3\n2\n1\n"},
		{`seq -w 8 10`, "08\n09\n10\n"},
		{`seq -s, 1 3`, "1,2,3\n"},
	}
	for _, test := range tests {
		result := run(t, New(), test.script)
		if result.ExitCode != 0 || result.Stdout != test.want {
			t.Fatalf("%s: %+v want %q", test.script, result, test.want)
		}
	}
}
