package gobash

import (
	"context"
	"strings"
	"testing"
)

func TestModernBash53LanguageCompatibility(t *testing.T) {
	result := run(t, New(), `
a=(zero one two)
sparse=([2]=two [5]=five)
declare -A mapped=([alpha]=one [beta]=two)
printf -v formatted '%s:%d\n\n' x 2
printf -v quoted '%q' 'a b'
case x in x) fall=a ;& y) fall+=b;; esac
printf '%s|%s|%s|%s|%s|%s|%s|%s\n' "${a[-1]}" "${#sparse[@]}" "${#mapped[@]}" "$((2#1010))" "$((010+1))" "$fall" "$quoted" "$formatted"
printf '%s\n' "${BASH_VERSINFO[*]}"`)
	const want = "two|2|2|10|9|ab|a\\ b|x:2\n\n\n5 3 15 1 go-bash go-bash\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("modern Bash compatibility: %+v want stdout %q", result, want)
	}
}

func TestDirectoryStackAndPrintfCompatibility(t *testing.T) {
	result := run(t, New(), `
mkdir -p /tmp/a /tmp/b
cd /tmp/a
pushd /tmp/b >/dev/null
dirs -p
popd >/dev/null
printf '%s|' "$PWD"
printf -- '-%s-|' x
printf -v out '%04d:%q' 7 'two words'
printf '%s|' "$out"
command printf '%q|' 'three words'
builtin printf '%s' done`)
	want := "/tmp/b\n/tmp/a\n/tmp/a|-x-|0007:two\\ words|three\\ words|done"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("directory stack/printf compatibility: %+v want %q", result, want)
	}
}

func TestInvalidOctalFailsInsteadOfSilentlyComputing(t *testing.T) {
	for _, script := range []string{`printf '%s' "$((08+1))"`, `n=08; printf '%s' "$((n+1))"`} {
		result := run(t, New(), script)
		if result.ExitCode == 0 || result.Stdout != "" || !strings.Contains(result.Stderr, "value too great for base") {
			t.Fatalf("invalid octal did not fail closed for %q: %+v", script, result)
		}
	}
	for _, test := range []struct{ script, want string }{
		{`n=08; if false; then printf '%s' "$((n+1))"; fi; printf 'ok=%s' "$n"`, "ok=08"},
		{`n=08; n=7; printf '%s' "$((n+1))"`, "8"},
	} {
		result := run(t, New(), test.script)
		if result.ExitCode != 0 || result.Stdout != test.want || result.Stderr != "" {
			t.Fatalf("path-sensitive octal guard for %q: %+v want %q", test.script, result, test.want)
		}
	}
}

func TestDynamicAssociativeArrayLength(t *testing.T) {
	result := run(t, New(), `declare -A m; m[alpha]=one; m[beta]=two; printf '%s' "${#m[@]}"`)
	if result.ExitCode != 0 || result.Stdout != "2" || result.Stderr != "" {
		t.Fatalf("dynamic associative-array length: %+v", result)
	}
}

func TestDevNullWorksForRedirectionsAndUtilities(t *testing.T) {
	result := run(t, New(), `printf discarded >/dev/null; cat /dev/null; type missing >/dev/null 2>&1 || printf missing`)
	if result.ExitCode != 0 || result.Stdout != "missing" || result.Stderr != "" {
		t.Fatalf("/dev/null compatibility: %+v", result)
	}
}

func TestFailedCdPreservesStatus(t *testing.T) {
	result := run(t, New(), `cd /does-not-exist || printf 'fallback:%s' "$?"`)
	if result.ExitCode != 0 || result.Stdout != "fallback:1" || !strings.Contains(result.Stderr, "no such file or directory") {
		t.Fatalf("failed cd status: %+v", result)
	}
}

func TestVirtualCwdAndAccessNeverConsultHost(t *testing.T) {
	sh := New(WithCwd("/virtual-start"))
	result := run(t, sh, `
printf '%s\n' "$PWD"
mkdir -p child
cd child
printf x >file
for op in -e -r -w; do test "$op" file || exit 10; done
test ! -x file || exit 11
test -x . || exit 12
printf '%s\n' "$PWD"`)
	if result.ExitCode != 0 || result.Stdout != "/virtual-start\n/virtual-start/child\n" || result.Stderr != "" {
		t.Fatalf("virtual cwd/access: %+v", result)
	}
}

func TestProcessSubstitutionFailsClosedAndLocationsRemainUserRelative(t *testing.T) {
	result, err := New().Run(context.Background(), `cat <(printf leaked)`)
	if err == nil || result.ExitCode != 2 || !strings.Contains(err.Error(), "process substitution is unavailable") || strings.Contains(err.Error(), "sh-interp") {
		t.Fatalf("process substitution should fail before host FIFO creation: result=%+v err=%v", result, err)
	}
	result, err = New().Run(context.Background(), "if then\n")
	if err == nil || result.ExitCode != 2 || !strings.Contains(err.Error(), "bash:1:1:") {
		t.Fatalf("parse location should be user-relative: result=%+v err=%v", result, err)
	}
	result = run(t, New(), `printf '%s' "$LINENO"`)
	if result.Stdout != "1" {
		t.Fatalf("LINENO was shifted by hidden prelude: %+v", result)
	}
}

func TestQuotaAndMissingParentWritesFailWithoutSilentDataLoss(t *testing.T) {
	result, err := New(WithFSQuota(8, 2)).Run(context.Background(), `printf 123456789 >/one; printf after`)
	if err != nil || result.ExitCode == 0 || !strings.Contains(result.Stderr, "quota exceeded") {
		t.Fatalf("quota failure should be structured: result=%+v err=%v", result, err)
	}
	sh := New()
	result = run(t, sh, `printf hi >/missing/child`)
	if result.ExitCode == 0 || result.Stderr == "" {
		t.Fatalf("missing parent redirect should fail: %+v", result)
	}
	result = run(t, sh, `test ! -e /missing/child`)
	if result.ExitCode != 0 {
		t.Fatalf("failed redirect created a file: %+v", result)
	}
}

func TestAgentUtilityWorkflows(t *testing.T) {
	result := run(t, New(), `
mkdir -p /tmp/r/sub
printf needle >/tmp/r/a.go
printf other >/tmp/r/b.txt
printf needle >/tmp/r/sub/c.go
cd /tmp/r
rg --files -g '*.go'
rg -n -g '*.go' needle
find . -type f -print0 | xargs -0 printf '<%s>\n'
printf 'a b\nc d\n' | xargs -I{} printf '[%s]\n' '{}'
printf '{}' | jq -c --arg name 'a b' --argjson data '{"x":1}' '{name:$name,data:$data}'
printf 'abc123\n' | sed -E 's/([a-z]+)([0-9]+)/\2-\1/'
printf 'abc\n' | sed -e 's/a/A/' -e 's/c/C/'
printf 'a b\tc\n' | tr -d '[:space:]'
printf '\n'
env TEMP_VALUE=ok printenv TEMP_VALUE
printf abc | base64 -w0
printf '\n'`)
	const want = "a.go\nsub/c.go\na.go:1:needle\nsub/c.go:1:needle\n" +
		"<./a.go>\n<./b.txt>\n<./sub/c.go>\n[a b]\n[c d]\n" +
		`{"data":{"x":1},"name":"a b"}` + "\n123-abc\nAbC\nabc\nok\nYWJj\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("agent utility workflows: %+v want stdout %q", result, want)
	}
}

func TestSafetyFlagsAreImplementedOrRejected(t *testing.T) {
	result := run(t, New(), `
printf old >/tmp/d; printf new >/tmp/s
cp -n /tmp/s /tmp/d
mv -n /tmp/s /tmp/d
printf '%s|' "$(cat /tmp/d)"
mkdir -m 700 /tmp/mode
test -d /tmp/mode && printf mode
rm -i /tmp/d
test -e /tmp/d && printf '|kept'`)
	if result.ExitCode != 0 || result.Stdout != "old|mode|kept" || !strings.Contains(result.Stderr, "rm: remove") {
		t.Fatalf("safety flags: %+v", result)
	}
	for _, script := range []string{
		`ls -R /tmp`, `sort -z`, `uniq -z`, `wc -q`, `touch -q /tmp/x`,
	} {
		result = run(t, New(), script)
		if result.ExitCode == 0 || !strings.Contains(result.Stderr, "unsupported option") {
			t.Fatalf("unsupported flag silently succeeded for %q: %+v", script, result)
		}
	}
}

func TestDestructivePathGuards(t *testing.T) {
	sh := New()
	result := run(t, sh, `
mkdir -p /tmp/a/sub /tmp/work
touch /tmp/a/f /tmp/work/keep
cp -r /tmp/a /tmp/a/sub/copied
mv /tmp/a /tmp/a/sub/moved
cd /tmp/work
rm -rf "" . .. /
test -f /tmp/a/f && test -f /tmp/work/keep`)
	if result.ExitCode != 0 || !strings.Contains(result.Stderr, "cannot copy a directory into itself") ||
		!strings.Contains(result.Stderr, "cannot move a directory into itself") ||
		!strings.Contains(result.Stderr, "dangerous") {
		t.Fatalf("destructive path guards: %+v", result)
	}
	result = run(t, sh, `find /tmp/a -type d | wc -l`)
	if strings.TrimSpace(result.Stdout) != "2" {
		t.Fatalf("failed copy left recursive partial data: %+v", result)
	}
}

func TestCommonResultInspectionSemantics(t *testing.T) {
	result := run(t, New(), `
seq 1 5 | tail -n +2
printf 'a\naa+\n' | grep 'a+'
printf 'before\nneedle\nafter\n' | grep -A 1 -B 1 -w needle
mkdir -p /tmp/r/vendor/deep /tmp/r/src
touch /tmp/r/vendor/deep/no.go /tmp/r/src/yes.go
rg --files -g '*.go' -g '!vendor/**' /tmp/r`)
	want := "2\n3\n4\n5\naa+\nbefore\nneedle\nafter\n/tmp/r/src/yes.go\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result inspection compatibility: %+v want %q", result, want)
	}
}
