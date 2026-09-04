package gobash

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/itchyny/gojq"
)

func init() { Register("jq", cmdJq) }

func cmdJq(ctx context.Context, e *Env) int {
	if len(e.Args) == 2 && e.Args[1] == "--help" {
		_, _ = fmt.Fprintln(e.Stdout, "usage: jq [-rscnSe] [--sort-keys] [--exit-status] filter [files...]")
		return 0
	}
	opts, query, operands, ok := parseJqArgs(e)
	if !ok {
		return 2
	}
	parsed, err := gojq.Parse(query)
	if err != nil {
		e.Errorf("compile error: %v", err)
		return 3
	}
	inputs := make([]any, 0)
	if opts.nullInput {
		inputs = append(inputs, nil)
	} else {
		code := forEachInput(ctx, e, operands, func(ctx context.Context, _ string, r io.Reader) error {
			dec := json.NewDecoder(r)
			dec.UseNumber()
			for {
				var value any
				if err := dec.Decode(&value); err != nil {
					if err == io.EOF {
						return nil
					}
					return err
				}
				inputs = append(inputs, value)
				if err := ctx.Err(); err != nil {
					return err
				}
			}
		})
		if code != 0 {
			return 4
		}
	}
	if opts.slurp {
		inputs = []any{inputs}
	}
	produced := false
	var lastValue any
	for _, input := range inputs {
		iter := parsed.RunWithContext(ctx, input)
		for {
			value, more := iter.Next()
			if !more {
				break
			}
			if err, isErr := value.(error); isErr {
				e.Errorf("%v", err)
				return 5
			}
			produced = true
			lastValue = value
			if opts.raw {
				if s, isString := value.(string); isString {
					if _, err := fmt.Fprintln(e.Stdout, s); err != nil {
						e.Errorf("%v", err)
						return 5
					}
					continue
				}
			}
			var encoded []byte
			var err error
			if opts.compact {
				encoded, err = json.Marshal(value)
			} else {
				encoded, err = json.MarshalIndent(value, "", "  ")
			}
			if err != nil {
				e.Errorf("encode result: %v", err)
				return 5
			}
			if _, err := fmt.Fprintln(e.Stdout, string(encoded)); err != nil {
				e.Errorf("%v", err)
				return 5
			}
		}
	}
	if opts.exitStatus {
		if !produced {
			return 4
		}
		if lastValue == nil || lastValue == false {
			return 1
		}
	}
	return 0
}

type jqOptions struct {
	raw        bool
	compact    bool
	slurp      bool
	nullInput  bool
	exitStatus bool
}

func parseJqArgs(e *Env) (opts jqOptions, query string, operands []string, ok bool) {
	args := e.Args[1:]
	for len(args) > 0 {
		a := args[0]
		if a == "--" {
			args = args[1:]
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			break
		}
		switch a {
		case "--raw-output":
			opts.raw = true
		case "--compact-output":
			opts.compact = true
		case "--slurp":
			opts.slurp = true
		case "--null-input":
			opts.nullInput = true
		case "--exit-status":
			opts.exitStatus = true
		case "--sort-keys", "--monochrome-output", "--color-output":
			// encoding/json sorts string map keys and color is never emitted.
		default:
			if strings.HasPrefix(a, "--") {
				e.Errorf("unsupported option: %s", a)
				return opts, "", nil, false
			}
			for _, flag := range strings.TrimPrefix(a, "-") {
				switch flag {
				case 'r':
					opts.raw = true
				case 'c':
					opts.compact = true
				case 's':
					opts.slurp = true
				case 'n':
					opts.nullInput = true
				case 'e':
					opts.exitStatus = true
				case 'S', 'M', 'C':
					// encoding/json sorts string map keys and color is never emitted.
				default:
					e.Errorf("unsupported option -- %c", flag)
					return opts, "", nil, false
				}
			}
		}
		args = args[1:]
	}
	if len(args) == 0 {
		e.Errorf("missing filter")
		return opts, "", nil, false
	}
	return opts, args[0], args[1:], true
}
