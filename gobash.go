package gobash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Shell is a reusable simulated bash environment. Its in-memory filesystem and
// environment persist across calls to [Shell.Run]. A Shell is safe for
// sequential use; it is not safe for concurrent Run calls.
type Shell struct {
	fs          afero.Fs
	cwd         string
	env         []string
	maxOutput   int
	maxScript   int
	maxCommands int
}

// Option configures a [Shell].
type Option func(*Shell)

// WithFS sets the filesystem backing the shell. Defaults to a fresh in-memory
// filesystem ([afero.NewMemMapFs]).
func WithFS(fs afero.Fs) Option { return func(s *Shell) { s.fs = fs } }

// WithCwd sets the initial working directory (must be absolute).
func WithCwd(dir string) Option { return func(s *Shell) { s.cwd = dir } }

// WithEnv sets an environment variable visible to scripts (KEY=value form is
// not required; pass key and value separately).
func WithEnv(key, value string) Option {
	return func(s *Shell) { s.env = append(s.env, key+"="+value) }
}

// WithLimits bounds captured output, script source, and external command
// invocations for each Run. Non-positive values retain the safe defaults.
func WithLimits(maxOutput, maxScript, maxCommands int) Option {
	return func(s *Shell) {
		if maxOutput > 0 {
			s.maxOutput = maxOutput
		}
		if maxScript > 0 {
			s.maxScript = maxScript
		}
		if maxCommands > 0 {
			s.maxCommands = maxCommands
		}
	}
}

// New creates a Shell. By default it uses a fresh in-memory filesystem rooted
// at "/", with no host environment leakage.
func New(opts ...Option) *Shell {
	s := &Shell{
		fs:          afero.NewMemMapFs(),
		cwd:         "/",
		env:         []string{"HOME=/root", "PWD=/"},
		maxOutput:   64 << 10,
		maxScript:   256 << 10,
		maxCommands: 256,
	}
	for _, o := range opts {
		o(s)
	}
	_ = s.fs.MkdirAll(s.cwd, 0o755)
	return s
}

// FS exposes the underlying virtual filesystem, e.g. to seed files before a run
// or inspect results afterwards.
func (s *Shell) FS() afero.Fs { return s.fs }

// Result is the outcome of a captured [Shell.Run].
type Result struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Truncated bool
}

// Run executes script, capturing stdout and stderr into the returned Result.
// The returned error is non-nil only for interpreter/parse failures, not for a
// non-zero exit status (which is reported via Result.ExitCode).
func (s *Shell) Run(ctx context.Context, script string) (Result, error) {
	stdout := newCaptureBuffer(s.maxOutput)
	stderr := newCaptureBuffer(s.maxOutput)
	code, err := s.RunIO(ctx, script, strings.NewReader(""), stdout, stderr)
	return Result{
		Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code,
		Truncated: stdout.truncated || stderr.truncated,
	}, err
}

type captureBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func newCaptureBuffer(limit int) *captureBuffer { return &captureBuffer{limit: limit} }

func (b *captureBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.truncated = b.truncated || original > 0
		return original, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, _ = b.Buffer.Write(p)
	return original, nil
}

func (b *captureBuffer) WriteString(s string) (int, error) { return b.Write([]byte(s)) }

// RunIO executes script with explicit stdin/stdout/stderr streams and returns
// the exit code. The error is non-nil only for parse/interpreter failures.
func (s *Shell) RunIO(ctx context.Context, script string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(script) > s.maxScript {
		return 2, fmt.Errorf("gobash: script exceeds %d-byte limit", s.maxScript)
	}
	ctx = context.WithValue(ctx, runStateKey{}, &runState{maxCommands: s.maxCommands})
	file, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	if err != nil {
		return 2, err
	}
	runner, err := interp.New(
		interp.StdIO(stdin, stdout, stderr),
		interp.Dir(s.cwd),
		interp.Env(expand.ListEnviron(s.env...)),
		interp.ExecHandlers(s.execMiddleware),
		interp.OpenHandler(s.openHandler),
		interp.StatHandler(s.statHandler),
		interp.ReadDirHandler2(s.readDirHandler),
	)
	if err != nil {
		return 2, err
	}
	runErr := runner.Run(ctx, file)
	if runErr == nil {
		return 0, nil
	}
	var status interp.ExitStatus
	if errors.As(runErr, &status) {
		return int(status), nil
	}
	return 1, runErr
}
