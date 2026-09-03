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
	raw, compact, slurp, nullInput, query, operands, ok := parseJqArgs(e)
	if !ok {
		return 2
	}
	parsed, err := gojq.Parse(query)
	if err != nil {
		e.Errorf("compile error: %v", err)
		return 3
	}
	inputs := make([]any, 0)
	if nullInput {
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
	if slurp {
		inputs = []any{inputs}
	}
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
			if raw {
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
			if compact {
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
	return 0
}

func parseJqArgs(e *Env) (raw, compact, slurp, nullInput bool, query string, operands []string, ok bool) {
	args := e.Args[1:]
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			args = args[i+1:]
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			args = args[i:]
			break
		}
		for _, flag := range strings.TrimPrefix(a, "-") {
			switch flag {
			case 'r':
				raw = true
			case 'c':
				compact = true
			case 's':
				slurp = true
			case 'n':
				nullInput = true
			case 'M', 'C', 'e':
				// Color is never emitted. -e is accepted for agent compatibility;
				// result truthiness does not alter the exit code yet.
			default:
				e.Errorf("unsupported option -- %c", flag)
				return false, false, false, false, "", nil, false
			}
		}
		if i == len(e.Args[1:])-1 {
			args = nil
		}
	}
	if len(args) == 0 {
		e.Errorf("missing filter")
		return false, false, false, false, "", nil, false
	}
	return raw, compact, slurp, nullInput, args[0], args[1:], true
}
