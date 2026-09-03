package gobash

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/afero"
)

func init() {
	Register("grep", cmdGrep)
	Register("egrep", cmdGrep)
	Register("fgrep", cmdGrep)
	Register("rg", cmdGrep)
}

type grepOptions struct {
	ignoreCase       bool
	invert           bool
	lineNumber       bool
	countOnly        bool
	filesWithMatches bool
	quiet            bool
	onlyMatching     bool
	fixed            bool
	recursive        bool
	showFilename     int // -1 never, 0 automatic, 1 always
	maxCount         int
	patterns         []string
	operands         []string
}

func cmdGrep(ctx context.Context, e *Env) int {
	opts, ok := parseGrepOptions(e)
	if !ok {
		return 2
	}
	if len(opts.patterns) == 0 {
		if len(opts.operands) == 0 {
			e.Errorf("missing search pattern")
			return 2
		}
		opts.patterns = append(opts.patterns, opts.operands[0])
		opts.operands = opts.operands[1:]
	}
	match, err := compileGrepMatcher(opts)
	if err != nil {
		e.Errorf("%v", err)
		return 2
	}
	inputs, err := expandGrepInputs(e, opts)
	if err != nil {
		e.Errorf("%v", err)
		return 2
	}
	showFilename := opts.showFilename > 0 || (opts.showFilename == 0 && len(inputs) > 1)
	found := false
	readErr := false
	if len(inputs) == 0 {
		inputs = []string{"-"}
	}
	for _, name := range inputs {
		var r io.Reader
		var closeFn func() error
		if name == "-" {
			r = e.Stdin
			closeFn = func() error { return nil }
		} else {
			f, openErr := e.FS.Open(e.Resolve(name))
			if openErr != nil {
				e.Errorf("%s: %v", name, openErr)
				readErr = true
				continue
			}
			r, closeFn = f, f.Close
		}
		matches := 0
		err := scanLines(ctx, r, func(line string, lineNo int) error {
			spans := match(line)
			lineMatches := len(spans) > 0
			if opts.invert {
				lineMatches = !lineMatches
			}
			if !lineMatches {
				return nil
			}
			found = true
			matches++
			if opts.quiet || opts.filesWithMatches || opts.countOnly {
				if opts.maxCount > 0 && matches >= opts.maxCount {
					return errGrepStop
				}
				return nil
			}
			prefix := ""
			if showFilename {
				prefix += name + ":"
			}
			if opts.lineNumber {
				prefix += strconv.Itoa(lineNo) + ":"
			}
			if opts.onlyMatching && !opts.invert {
				for _, span := range spans {
					fmt.Fprintln(e.Stdout, prefix+line[span[0]:span[1]])
				}
			} else {
				fmt.Fprintln(e.Stdout, prefix+line)
			}
			if opts.maxCount > 0 && matches >= opts.maxCount {
				return errGrepStop
			}
			return nil
		})
		_ = closeFn()
		if err != nil && !errors.Is(err, errGrepStop) {
			e.Errorf("%s: %v", name, err)
			readErr = true
		}
		if opts.filesWithMatches && matches > 0 {
			fmt.Fprintln(e.Stdout, name)
		}
		if opts.countOnly {
			if showFilename {
				fmt.Fprintf(e.Stdout, "%s:", name)
			}
			fmt.Fprintln(e.Stdout, matches)
		}
		if opts.quiet && found {
			return 0
		}
	}
	if readErr {
		return 2
	}
	if found {
		return 0
	}
	return 1
}

var errGrepStop = errors.New("grep max count reached")

func parseGrepOptions(e *Env) (grepOptions, bool) {
	opts := grepOptions{maxCount: -1}
	args := e.Args[1:]
	if e.Args[0] == "fgrep" {
		opts.fixed = true
	}
	if e.Args[0] == "rg" {
		opts.recursive = true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			opts.operands = append(opts.operands, args[i+1:]...)
			break
		}
		if a == "-e" || a == "--regexp" || a == "-m" || a == "--max-count" {
			if i+1 >= len(args) {
				e.Errorf("option %s requires an argument", a)
				return opts, false
			}
			i++
			if a == "-e" || a == "--regexp" {
				opts.patterns = append(opts.patterns, args[i])
			} else {
				n, err := strconv.Atoi(args[i])
				if err != nil || n < 0 {
					e.Errorf("invalid max count: %s", args[i])
					return opts, false
				}
				opts.maxCount = n
			}
			continue
		}
		if len(a) > 1 && strings.HasPrefix(a, "-") {
			for _, flag := range a[1:] {
				switch flag {
				case 'i':
					opts.ignoreCase = true
				case 'v':
					opts.invert = true
				case 'n':
					opts.lineNumber = true
				case 'c':
					opts.countOnly = true
				case 'l':
					opts.filesWithMatches = true
				case 'q':
					opts.quiet = true
				case 'o':
					opts.onlyMatching = true
				case 'F':
					opts.fixed = true
				case 'E':
					opts.fixed = false
				case 'r', 'R':
					opts.recursive = true
				case 'h':
					opts.showFilename = -1
				case 'H':
					opts.showFilename = 1
				case 's': // accepted; diagnostics remain intentionally concise
				default:
					e.Errorf("unsupported option -- %c", flag)
					return opts, false
				}
			}
			continue
		}
		opts.operands = append(opts.operands, a)
	}
	return opts, true
}

func compileGrepMatcher(opts grepOptions) (func(string) [][2]int, error) {
	patterns := make([]string, len(opts.patterns))
	for i, pattern := range opts.patterns {
		if opts.fixed {
			pattern = regexp.QuoteMeta(pattern)
		}
		patterns[i] = "(?:" + pattern + ")"
	}
	pattern := strings.Join(patterns, "|")
	if opts.ignoreCase {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return func(s string) [][2]int {
		raw := re.FindAllStringIndex(s, -1)
		spans := make([][2]int, len(raw))
		for i := range raw {
			spans[i] = [2]int{raw[i][0], raw[i][1]}
		}
		return spans
	}, nil
}

func expandGrepInputs(e *Env, opts grepOptions) ([]string, error) {
	if len(opts.operands) == 0 {
		return nil, nil
	}
	var out []string
	for _, operand := range opts.operands {
		isDir, err := afero.IsDir(e.FS, e.Resolve(operand))
		if err != nil || !isDir {
			out = append(out, operand)
			continue
		}
		if !opts.recursive {
			return nil, fmt.Errorf("%s: Is a directory", operand)
		}
		root := e.Resolve(operand)
		err = afero.Walk(e.FS, root, func(p string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				rel := strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
				if operand == "." {
					out = append(out, rel)
				} else {
					out = append(out, strings.TrimRight(operand, "/")+"/"+rel)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
