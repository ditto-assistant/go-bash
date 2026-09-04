package gobash

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

func init() { Register("base64", cmdBase64) }

func cmdBase64(ctx context.Context, e *Env) int {
	decode, noWrap, operands, ok := parseBase64Args(e)
	if !ok {
		return 2
	}
	return forEachInput(ctx, e, operands, func(ctx context.Context, _ string, r io.Reader) error {
		if decode {
			data, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			decoded, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(data)), ""))
			if err != nil {
				return err
			}
			_, err = e.Stdout.Write(decoded)
			return err
		}
		encoder := base64.NewEncoder(base64.StdEncoding, e.Stdout)
		if _, err := io.Copy(encoder, r); err != nil {
			_ = encoder.Close()
			return err
		}
		if err := encoder.Close(); err != nil {
			return err
		}
		if !noWrap {
			if _, err := fmt.Fprintln(e.Stdout); err != nil {
				return err
			}
		}
		return ctx.Err()
	})
}

func parseBase64Args(e *Env) (decode, noWrap bool, operands []string, ok bool) {
	args := e.Args[1:]
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--":
			return decode, noWrap, append(operands, args[i+1:]...), true
		case args[i] == "-d" || args[i] == "-D" || args[i] == "--decode":
			decode = true
		case args[i] == "-w" || args[i] == "--wrap":
			if i+1 >= len(args) || args[i+1] != "0" {
				e.Errorf("only unwrapped output (-w 0) is supported")
				return false, false, nil, false
			}
			noWrap = true
			i++
		case strings.HasPrefix(args[i], "-w") && len(args[i]) > 2:
			if args[i][2:] != "0" {
				e.Errorf("only unwrapped output (-w 0) is supported")
				return false, false, nil, false
			}
			noWrap = true
		case strings.HasPrefix(args[i], "--wrap="):
			if strings.TrimPrefix(args[i], "--wrap=") != "0" {
				e.Errorf("only unwrapped output (-w 0) is supported")
				return false, false, nil, false
			}
			noWrap = true
		case strings.HasPrefix(args[i], "-") && args[i] != "-":
			e.Errorf("unsupported option: %s", args[i])
			return false, false, nil, false
		default:
			operands = append(operands, args[i])
		}
	}
	return decode, noWrap, operands, true
}
