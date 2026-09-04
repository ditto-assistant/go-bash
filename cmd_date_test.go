package gobash

import (
	"testing"
	"time"
)

func TestDateFormattingAndMath(t *testing.T) {
	fixed := time.Date(2026, time.September, 4, 12, 34, 56, 0, time.FixedZone("EDT", -4*60*60))
	sh := New(WithNow(func() time.Time { return fixed }))
	tests := []struct {
		script string
		want   string
	}{
		{`date -u +%Y-%m-%dT%H:%M:%SZ`, "2026-09-04T16:34:56Z\n"},
		{`date -d '2 days' +%F`, "2026-09-06\n"},
		{`date -d '1 hour ago' +%T`, "11:34:56\n"},
		{`date -u -d @0 +%s`, "0\n"},
		{`date -d '2026-09-04T12:00:00Z' +%FT%TZ`, "2026-09-04T12:00:00Z\n"},
	}
	for _, test := range tests {
		result := run(t, sh, test.script)
		if result.ExitCode != 0 || result.Stdout != test.want {
			t.Fatalf("%s: %+v want %q", test.script, result, test.want)
		}
	}
}

func TestDateUTCAnchoredArithmeticUsesUTCForNaiveAnchor(t *testing.T) {
	centralEuropeanTime := time.FixedZone("CET", 60*60)
	fixed := time.Date(2026, time.September, 4, 12, 34, 56, 0, centralEuropeanTime)
	result := run(t, New(WithNow(func() time.Time { return fixed })), `date -u -d '2026-01-01 + 1 day'`)
	if result.ExitCode != 0 || result.Stdout != "Fri Jan  2 00:00:00 UTC 2026\n" || result.Stderr != "" {
		t.Fatalf("UTC anchored arithmetic: %+v", result)
	}
}
