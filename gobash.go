package gobash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

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
	maxFSBytes  int64
	maxFSFiles  int
	timeout     time.Duration
	now         func() time.Time
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

// WithFSQuota bounds the aggregate bytes and regular-file count in the virtual
// filesystem. Non-positive values retain the safe defaults.
func WithFSQuota(maxBytes int64, maxFiles int) Option {
	return func(s *Shell) {
		if maxBytes > 0 {
			s.maxFSBytes = maxBytes
		}
		if maxFiles > 0 {
			s.maxFSFiles = maxFiles
		}
	}
}

// WithTimeout bounds each Run or RunIO invocation. The default is five
// seconds. Callers may still provide an earlier deadline through the context.
func WithTimeout(timeout time.Duration) Option {
	return func(s *Shell) {
		if timeout > 0 {
			s.timeout = timeout
		}
	}
}

// WithNow supplies the clock used by time-aware commands such as date. It is
// primarily useful for deterministic tests.
func WithNow(now func() time.Time) Option {
	return func(s *Shell) {
		if now != nil {
			s.now = now
		}
	}
}

// New creates a Shell. By default it uses a fresh in-memory filesystem rooted
// at "/" with a virtual working directory of "/tmp" and no host environment
// leakage.
func New(opts ...Option) *Shell {
	s := &Shell{
		fs:  afero.NewMemMapFs(),
		cwd: "/tmp",
		env: []string{
			"GOBASH_RUNTIME=mvdan-sh",
			"HOME=/tmp",
			"LOGNAME=agent",
			"PATH=/gobash/bin",
			"PWD=/tmp",
			"SHELL=bash",
			"USER=agent",
		},
		maxOutput:   64 << 10,
		maxScript:   256 << 10,
		maxCommands: 256,
		maxFSBytes:  64 << 20,
		maxFSFiles:  1024,
		timeout:     5 * time.Second,
		now:         time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	s.fs = newQuotaFS(s.fs, s.maxFSBytes, s.maxFSFiles)
	s.cwd = path.Clean(s.cwd)
	_ = s.fs.MkdirAll(s.cwd, 0o755)
	_ = s.fs.MkdirAll("/tmp", 0o700)
	return s
}

// FS exposes the underlying virtual filesystem, e.g. to seed files before a run
// or inspect results afterwards.
func (s *Shell) FS() afero.Fs { return s.fs }

// Result is the outcome of a captured [Shell.Run].
type Result struct {
	Stdout          string
	Stderr          string
	ExitCode        int
	Truncated       bool
	StdoutTruncated bool
	StderrTruncated bool
	Shell           string
	Runtime         string
	Cwd             string
	TimeoutMS       int64
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
		Truncated:       stdout.truncated || stderr.truncated,
		StdoutTruncated: stdout.truncated, StderrTruncated: stderr.truncated,
		Shell: "bash", Runtime: "go-bash/mvdan-sh", Cwd: s.cwd,
		TimeoutMS: s.timeout.Milliseconds(),
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
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	ctx = context.WithValue(ctx, runStateKey{}, &runState{maxCommands: s.maxCommands})
	file, err := syntax.NewParser().Parse(strings.NewReader(commandPrelude()+script), "bash")
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
	if errors.Is(runErr, context.DeadlineExceeded) {
		_, _ = fmt.Fprintln(stderr, "bash: execution timed out")
		return 124, nil
	}
	if errors.Is(runErr, context.Canceled) {
		_, _ = fmt.Fprintln(stderr, "bash: execution canceled")
		return 130, nil
	}
	return 1, runErr
}
