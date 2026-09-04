package gobash

import (
	"context"
	"testing"
	"time"
)

func TestFindCommonPredicateForms(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name: "explicit print after maxdepth",
			script: `d=/tmp/find-test
mkdir -p "$d/sub"
touch "$d/a.json" "$d/b.txt" "$d/sub/nested.json"
find "$d" -type f -maxdepth 1 -print`,
			want: "/tmp/find-test/a.json\n/tmp/find-test/b.txt\n",
		},
		{
			name: "maxdepth before type",
			script: `mkdir -p /tmp/find-test/sub
touch /tmp/find-test/a.json /tmp/find-test/sub/nested.json
find /tmp/find-test -maxdepth 1 -type f`,
			want: "/tmp/find-test/a.json\n",
		},
		{
			name: "name filter",
			script: `mkdir -p /tmp/find-test
touch /tmp/find-test/a.json /tmp/find-test/b.txt
find /tmp/find-test -name '*.json'`,
			want: "/tmp/find-test/a.json\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := New().Run(context.Background(), test.script)
			if err != nil || result.ExitCode != 0 || result.Stderr != "" || result.Stdout != test.want {
				t.Fatalf("result=%+v err=%v want_stdout=%q", result, err, test.want)
			}
		})
	}
}

func TestJqSortedKeysCompatibility(t *testing.T) {
	const input = `{"z":1,"a":{"d":4,"b":2}}`
	tests := []struct {
		flag string
		want string
	}{
		{
			flag: "-S",
			want: "{\n  \"a\": {\n    \"b\": 2,\n    \"d\": 4\n  },\n  \"z\": 1\n}\n",
		},
		{flag: "-Sc", want: `{"a":{"b":2,"d":4},"z":1}` + "\n"},
		{flag: "--sort-keys", want: "{\n  \"a\": {\n    \"b\": 2,\n    \"d\": 4\n  },\n  \"z\": 1\n}\n"},
	}
	for _, test := range tests {
		t.Run(test.flag, func(t *testing.T) {
			result, err := New().Run(context.Background(), `printf '%s' '`+input+`' | jq `+test.flag+` .`)
			if err != nil || result.ExitCode != 0 || result.Stderr != "" || result.Stdout != test.want {
				t.Fatalf("result=%+v err=%v want_stdout=%q", result, err, test.want)
			}
		})
	}
}

func TestDateAnchoredArithmeticCompatibility(t *testing.T) {
	fixed := time.Date(2026, time.September, 4, 12, 34, 56, 0, time.UTC)
	sh := New(WithNow(func() time.Time { return fixed }))
	tests := []struct {
		script string
		want   string
	}{
		{`date -d 'tomorrow' +%F`, "2026-09-05\n"},
		{`date -d '2026-01-01 + 1 day' +%F`, "2026-01-02\n"},
		{`date -d '2026-01-01 - 2 weeks' +%F`, "2025-12-18\n"},
		{`date -d '2026-01-01 12:30:00 + 90 minutes' +%FT%T`, "2026-01-01T14:00:00\n"},
		{`date -u -d '@ 1767225600' +%FT%TZ`, "2026-01-01T00:00:00Z\n"},
	}
	for _, test := range tests {
		result, err := sh.Run(context.Background(), test.script)
		if err != nil || result.ExitCode != 0 || result.Stderr != "" || result.Stdout != test.want {
			t.Fatalf("%s: result=%+v err=%v want_stdout=%q", test.script, result, err, test.want)
		}
	}
}
