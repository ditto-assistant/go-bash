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
	flags, operands := splitArgs(e.Args[1:])
	decode := flags['d'] || flags['D']
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
		fmt.Fprintln(e.Stdout)
		return ctx.Err()
	})
}
