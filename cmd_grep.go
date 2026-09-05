package gobash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
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
	extended         bool
	wordRegexp       bool
	recursive        bool
	beforeContext    int
	afterContext     int
	showFilename     int // -1 never, 0 automatic, 1 always
	maxCount         int
	filesOnly        bool
	globs            []string
	patterns         []string
	operands         []string
}

func cmdGrep(ctx context.Context, e *Env) int {
	opts, ok := parseGrepOptions(e)
	if !ok {
		return 2
	}
	if opts.filesOnly {
		return listRipgrepFiles(ctx, e, opts)
	}
	if len(opts.patterns) == 0 {
		if len(opts.operands) == 0 {
			e.Errorf("missing search pattern")
			return 2
		}
		opts.patterns = append(opts.patterns, opts.operands[0])
		opts.operands = opts.operands[1:]
	}
	if e.Args[0] == "rg" && len(opts.operands) == 0 {
		// The interpreter is non-interactive, but preserves ripgrep's useful
		// terminal behavior by falling back to recursive VFS search when the
		// run-level stdin is empty. Peek one byte so an actual pipeline wins
		// even when the working directory already contains files.
		var first [1]byte
		n, readErr := e.Stdin.Read(first[:])
		if n > 0 {
			e.Stdin = io.MultiReader(bytes.NewReader(first[:n]), e.Stdin)
		} else if errors.Is(readErr, io.EOF) {
			opts.operands = []string{"."}
		} else if readErr != nil {
			e.Errorf("read stdin: %v", readErr)
			return 1
		}
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
	recursiveSearch := false
	for _, operand := range opts.operands {
		if isDir, _ := afero.IsDir(e.FS, e.Resolve(operand)); isDir {
			recursiveSearch = true
			break
		}
	}
	showFilename := opts.showFilename > 0 || (opts.showFilename == 0 && (len(inputs) > 1 || e.Args[0] == "rg" && recursiveSearch && len(inputs) > 0))
	if (opts.beforeContext > 0 || opts.afterContext > 0) && !opts.onlyMatching {
		return runContextGrep(ctx, e, opts, inputs, match, showFilename)
	}
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
					if _, err := fmt.Fprintln(e.Stdout, prefix+line[span[0]:span[1]]); err != nil {
						return err
					}
				}
			} else {
				if _, err := fmt.Fprintln(e.Stdout, prefix+line); err != nil {
					return err
				}
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
			if _, err := fmt.Fprintln(e.Stdout, name); err != nil {
				readErr = true
			}
		}
		if opts.countOnly {
			if showFilename {
				if _, err := fmt.Fprintf(e.Stdout, "%s:", name); err != nil {
					readErr = true
					continue
				}
			}
			if _, err := fmt.Fprintln(e.Stdout, matches); err != nil {
				readErr = true
			}
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
		opts.extended = true
	}
	if e.Args[0] == "egrep" {
		opts.extended = true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			opts.operands = append(opts.operands, args[i+1:]...)
			break
		}
		if a == "--files" && e.Args[0] == "rg" {
			opts.filesOnly = true
			continue
		}
		if a == "-g" || a == "--glob" {
			if e.Args[0] != "rg" || i+1 >= len(args) {
				e.Errorf("option %s requires an argument", a)
				return opts, false
			}
			i++
			opts.globs = append(opts.globs, args[i])
			continue
		}
		if a == "-A" || a == "--after-context" || a == "-B" || a == "--before-context" || a == "-C" || a == "--context" {
			if i+1 >= len(args) {
				e.Errorf("option %s requires an argument", a)
				return opts, false
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 {
				e.Errorf("invalid context length: %s", args[i])
				return opts, false
			}
			switch a {
			case "-A", "--after-context":
				opts.afterContext = n
			case "-B", "--before-context":
				opts.beforeContext = n
			default:
				opts.beforeContext, opts.afterContext = n, n
			}
			continue
		}
		if len(a) > 2 && a[0] == '-' && strings.ContainsRune("ABC", rune(a[1])) && allDigits(a[2:]) {
			n, _ := strconv.Atoi(a[2:])
			switch a[1] {
			case 'A':
				opts.afterContext = n
			case 'B':
				opts.beforeContext = n
			case 'C':
				opts.beforeContext, opts.afterContext = n, n
			}
			continue
		}
		if strings.HasPrefix(a, "--glob=") && e.Args[0] == "rg" {
			opts.globs = append(opts.globs, strings.TrimPrefix(a, "--glob="))
			continue
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
					opts.extended = true
				case 'G':
					opts.fixed = false
					opts.extended = false
				case 'w':
					opts.wordRegexp = true
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
		} else if !opts.extended {
			pattern = sedBREToGo(pattern)
		}
		if opts.wordRegexp {
			pattern = `\b(?:` + pattern + `)\b`
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

func runContextGrep(ctx context.Context, e *Env, opts grepOptions, inputs []string, match func(string) [][2]int, showFilename bool) int {
	found, readErr := false, false
	if len(inputs) == 0 {
		inputs = []string{"-"}
	}
	for _, name := range inputs {
		var r io.Reader
		var closeFn func() error
		if name == "-" {
			r, closeFn = e.Stdin, func() error { return nil }
		} else {
			f, err := e.FS.Open(e.Resolve(name))
			if err != nil {
				e.Errorf("%s: %v", name, err)
				readErr = true
				continue
			}
			r, closeFn = f, f.Close
		}
		var lines []string
		if err := scanLines(ctx, r, func(line string, _ int) error { lines = append(lines, line); return nil }); err != nil {
			e.Errorf("%s: %v", name, err)
			readErr = true
		}
		_ = closeFn()
		matched := make([]bool, len(lines))
		matches := 0
		for i, line := range lines {
			lineMatches := len(match(line)) > 0
			if opts.invert {
				lineMatches = !lineMatches
			}
			if lineMatches && (opts.maxCount < 0 || matches < opts.maxCount) {
				matched[i] = true
				matches++
				found = true
			}
		}
		if opts.quiet && matches > 0 {
			return 0
		}
		if opts.filesWithMatches {
			if matches > 0 {
				_, _ = fmt.Fprintln(e.Stdout, name)
			}
			continue
		}
		if opts.countOnly {
			if showFilename {
				_, _ = fmt.Fprintf(e.Stdout, "%s:", name)
			}
			_, _ = fmt.Fprintln(e.Stdout, matches)
			continue
		}
		selected := make([]bool, len(lines))
		for i, yes := range matched {
			if !yes {
				continue
			}
			from, to := max(0, i-opts.beforeContext), min(len(lines)-1, i+opts.afterContext)
			for j := from; j <= to; j++ {
				selected[j] = true
			}
		}
		last := -2
		for i, yes := range selected {
			if !yes {
				continue
			}
			if last >= 0 && i > last+1 {
				_, _ = fmt.Fprintln(e.Stdout, "--")
			}
			sep := "-"
			if matched[i] {
				sep = ":"
			}
			prefix := ""
			if showFilename {
				prefix += name + sep
			}
			if opts.lineNumber {
				prefix += strconv.Itoa(i+1) + sep
			}
			if _, err := fmt.Fprintln(e.Stdout, prefix+lines[i]); err != nil {
				readErr = true
			}
			last = i
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
				if !matchesGrepGlobs(rel, opts.globs) {
					return nil
				}
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

func listRipgrepFiles(ctx context.Context, e *Env, opts grepOptions) int {
	roots := opts.operands
	if len(roots) == 0 {
		roots = []string{"."}
	}
	inputs, err := expandGrepInputs(e, grepOptions{recursive: true, operands: roots, globs: opts.globs})
	if err != nil {
		e.Errorf("%v", err)
		return 2
	}
	for _, name := range inputs {
		if err := ctx.Err(); err != nil {
			return 124
		}
		if _, err := fmt.Fprintln(e.Stdout, name); err != nil {
			e.Errorf("%v", err)
			return 1
		}
	}
	return 0
}

func matchesGrepGlobs(name string, globs []string) bool {
	if len(globs) == 0 {
		return true
	}
	included, hasInclude := false, false
	for _, glob := range globs {
		exclude := strings.HasPrefix(glob, "!")
		if exclude {
			glob = strings.TrimPrefix(glob, "!")
		} else {
			hasInclude = true
		}
		matched := matchAgentGlob(glob, name)
		if !matched {
			matched = matchAgentGlob(glob, path.Base(name))
		}
		if matched && exclude {
			return false
		}
		if matched {
			included = true
		}
	}
	return included || !hasInclude
}

// matchAgentGlob extends path.Match with the globstar form agents commonly use
// with ripgrep, e.g. !vendor/** and **/*.json.
func matchAgentGlob(pattern, name string) bool {
	if !strings.Contains(pattern, "**") {
		matched, _ := path.Match(pattern, name)
		return matched
	}
	var expression strings.Builder
	expression.WriteByte('^')
	for i := 0; i < len(pattern); {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			expression.WriteString(`(?:.*/)?`)
			i += 3
		case strings.HasPrefix(pattern[i:], "**"):
			expression.WriteString(`.*`)
			i += 2
		case pattern[i] == '*':
			expression.WriteString(`[^/]*`)
			i++
		case pattern[i] == '?':
			expression.WriteString(`[^/]`)
			i++
		default:
			expression.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
			i++
		}
	}
	expression.WriteByte('$')
	re, err := regexp.Compile(expression.String())
	return err == nil && re.MatchString(name)
}
