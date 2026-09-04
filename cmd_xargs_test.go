package gobash

import "testing"

func TestXargsBatchesAndInvokesRegisteredCommands(t *testing.T) {
	tests := []struct {
		script string
		want   string
	}{
		{`printf 'a b c' | xargs -n2`, "a b\nc\n"},
		{`printf 'b\na\n' | xargs -n1 basename`, "b\na\n"},
		{`printf '/a\0/b\0' | xargs -0 -n1 dirname`, "/\n/\n"},
		{`printf 'a\nb\n' | xargs -I{} basename '/root/{}.txt' .txt`, "a\nb\n"},
	}
	for _, test := range tests {
		result := run(t, New(), test.script)
		if result.ExitCode != 0 || result.Stdout != test.want {
			t.Fatalf("%s: %+v want %q", test.script, result, test.want)
		}
	}
}

func TestXargsInvokesShellBuiltins(t *testing.T) {
	result := run(t, New(), `printf 'a\nb\n' | xargs printf '[%s]'`)
	if result.ExitCode != 0 || result.Stdout != "[a][b]" || result.Stderr != "" {
		t.Fatalf("xargs shell builtin: %+v", result)
	}
}
