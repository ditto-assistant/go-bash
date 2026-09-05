package gobash

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func init() {
	Register("hexdump", cmdHexdump)
	Register("od", cmdOd)
	Register("xxd", cmdXxd)
}

type byteReadOptions struct {
	limit    int64
	skip     int64
	operands []string
}

func readBytes(ctx context.Context, e *Env, opts byteReadOptions) ([]byte, int) {
	var result []byte
	code := forEachInput(ctx, e, opts.operands, func(ctx context.Context, _ string, reader io.Reader) error {
		if opts.skip > 0 {
			if _, err := io.CopyN(io.Discard, reader, opts.skip); err != nil && !errors.Is(err, io.EOF) {
				return err
			}
		}
		if opts.limit >= 0 {
			reader = io.LimitReader(reader, opts.limit)
		}
		data, err := io.ReadAll(&contextReader{ctx: ctx, reader: reader})
		result = append(result, data...)
		return err
	})
	return result, code
}

type xxdOptions struct {
	byteReadOptions
	plain   bool
	reverse bool
	columns int
}

// cmdXxd supports canonical and plain dumps, -r -p round trips, -l, -s, and -c.
func cmdXxd(ctx context.Context, e *Env) int {
	if len(e.Args) == 2 && e.Args[1] == "--help" {
		_, _ = fmt.Fprintln(e.Stdout, "usage: xxd [-p] [-r] [-l LEN] [-s OFFSET] [-c COLS] [file]")
		return 0
	}
	opts, ok := parseXxdArgs(e)
	if !ok {
		return 2
	}
	data, code := readBytes(ctx, e, opts.byteReadOptions)
	if code != 0 {
		return code
	}
	if opts.reverse {
		if !opts.plain {
			e.Errorf("reverse mode currently requires -p")
			return 2
		}
		cleaned := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
				return -1
			}
			return r
		}, string(data))
		decoded, err := hex.DecodeString(cleaned)
		if err != nil {
			e.Errorf("invalid hex input: %v", err)
			return 1
		}
		_, _ = e.Stdout.Write(decoded)
		return 0
	}
	if opts.plain {
		lineBytes := opts.columns
		if lineBytes <= 0 {
			lineBytes = 30
		}
		for start := 0; start < len(data); start += lineBytes {
			end := min(start+lineBytes, len(data))
			_, _ = fmt.Fprintln(e.Stdout, hex.EncodeToString(data[start:end]))
		}
		return 0
	}
	columns := opts.columns
	if columns <= 0 {
		columns = 16
	}
	for start := 0; start < len(data); start += columns {
		end := min(start+columns, len(data))
		chunk := data[start:end]
		var groups strings.Builder
		for i := 0; i < columns; i += 2 {
			if i > 0 {
				groups.WriteByte(' ')
			}
			if i < len(chunk) {
				_, _ = fmt.Fprintf(&groups, "%02x", chunk[i])
			}
			if i+1 < len(chunk) {
				_, _ = fmt.Fprintf(&groups, "%02x", chunk[i+1])
			}
		}
		hexWidth := columns*2 + (columns-1)/2
		_, _ = fmt.Fprintf(e.Stdout, "%08x: %-*s  %s\n", start+int(opts.skip), hexWidth, groups.String(), printableBytes(chunk))
	}
	return 0
}

func parseXxdArgs(e *Env) (xxdOptions, bool) {
	opts := xxdOptions{byteReadOptions: byteReadOptions{limit: -1}}
	args := e.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--":
			opts.operands = append(opts.operands, args[i+1:]...)
			return validateSingleOperand(e, opts)
		case "-p", "-plain":
			opts.plain = true
		case "-r", "-revert":
			opts.reverse = true
		case "-l", "-len", "-s", "-seek", "-c", "-cols":
			if i+1 >= len(args) {
				e.Errorf("option %s requires an argument", args[i])
				return xxdOptions{}, false
			}
			flag := args[i]
			i++
			value, err := strconv.ParseInt(args[i], 0, 64)
			if err != nil || value < 0 {
				e.Errorf("invalid numeric value: %s", args[i])
				return xxdOptions{}, false
			}
			switch flag {
			case "-l", "-len":
				opts.limit = value
			case "-s", "-seek":
				opts.skip = value
			default:
				if value < 1 || value > 256 {
					e.Errorf("invalid column count: %d", value)
					return xxdOptions{}, false
				}
				opts.columns = int(value)
			}
		default:
			if strings.HasPrefix(args[i], "-") && args[i] != "-" {
				e.Errorf("unsupported option: %s", args[i])
				return xxdOptions{}, false
			}
			opts.operands = append(opts.operands, args[i])
		}
	}
	return validateSingleOperand(e, opts)
}

func validateSingleOperand(e *Env, opts xxdOptions) (xxdOptions, bool) {
	if len(opts.operands) > 1 {
		e.Errorf("only one input file is supported")
		return xxdOptions{}, false
	}
	return opts, true
}

type odOptions struct {
	byteReadOptions
	noAddress bool
}

// cmdOd implements the common agent byte probe `od -An -tx1`, plus -N and -j.
func cmdOd(ctx context.Context, e *Env) int {
	if len(e.Args) == 2 && e.Args[1] == "--help" {
		_, _ = fmt.Fprintln(e.Stdout, "usage: od [-An] [-tx1] [-N BYTES] [-j BYTES] [file]")
		return 0
	}
	opts, ok := parseOdArgs(e)
	if !ok {
		return 2
	}
	data, code := readBytes(ctx, e, opts.byteReadOptions)
	if code != 0 {
		return code
	}
	for start := 0; start < len(data); start += 16 {
		end := min(start+16, len(data))
		if !opts.noAddress {
			_, _ = fmt.Fprintf(e.Stdout, "%07o", int(opts.skip)+start)
		}
		for _, value := range data[start:end] {
			_, _ = fmt.Fprintf(e.Stdout, " %02x", value)
		}
		_, _ = fmt.Fprintln(e.Stdout)
	}
	if !opts.noAddress {
		_, _ = fmt.Fprintf(e.Stdout, "%07o\n", int(opts.skip)+len(data))
	}
	return 0
}

func parseOdArgs(e *Env) (odOptions, bool) {
	opts := odOptions{byteReadOptions: byteReadOptions{limit: -1}}
	args := e.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-An" || arg == "-A" && i+1 < len(args) && args[i+1] == "n":
			opts.noAddress = true
			if arg == "-A" {
				i++
			}
		case arg == "-tx1" || arg == "-t" && i+1 < len(args) && args[i+1] == "x1":
			if arg == "-t" {
				i++
			}
		case arg == "-v":
		case arg == "-N" || arg == "-j":
			if i+1 >= len(args) {
				e.Errorf("option %s requires an argument", arg)
				return odOptions{}, false
			}
			i++
			value, err := strconv.ParseInt(args[i], 0, 64)
			if err != nil || value < 0 {
				e.Errorf("invalid byte count: %s", args[i])
				return odOptions{}, false
			}
			if arg == "-N" {
				opts.limit = value
			} else {
				opts.skip = value
			}
		case strings.HasPrefix(arg, "-") && arg != "-":
			e.Errorf("unsupported option: %s", arg)
			return odOptions{}, false
		default:
			opts.operands = append(opts.operands, arg)
		}
	}
	if len(opts.operands) > 1 {
		e.Errorf("only one input file is supported")
		return odOptions{}, false
	}
	return opts, true
}

// cmdHexdump implements canonical -C output with -n and -s bounds.
func cmdHexdump(ctx context.Context, e *Env) int {
	if len(e.Args) == 2 && e.Args[1] == "--help" {
		_, _ = fmt.Fprintln(e.Stdout, "usage: hexdump -C [-n BYTES] [-s BYTES] [file]")
		return 0
	}
	opts := byteReadOptions{limit: -1}
	canonical := false
	args := e.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-C", "--canonical":
			canonical = true
		case "-v":
		case "-n", "-s":
			if i+1 >= len(args) {
				e.Errorf("option %s requires an argument", args[i])
				return 2
			}
			flag := args[i]
			i++
			value, err := strconv.ParseInt(args[i], 0, 64)
			if err != nil || value < 0 {
				e.Errorf("invalid byte count: %s", args[i])
				return 2
			}
			if flag == "-n" {
				opts.limit = value
			} else {
				opts.skip = value
			}
		default:
			if strings.HasPrefix(args[i], "-") && args[i] != "-" {
				e.Errorf("unsupported option: %s", args[i])
				return 2
			}
			opts.operands = append(opts.operands, args[i])
		}
	}
	if !canonical {
		e.Errorf("only canonical -C output is supported")
		return 2
	}
	if len(opts.operands) > 1 {
		e.Errorf("only one input file is supported")
		return 2
	}
	data, code := readBytes(ctx, e, opts)
	if code != 0 {
		return code
	}
	if len(data) == 0 {
		return 0
	}
	writer := bufio.NewWriter(e.Stdout)
	for start := 0; start < len(data); start += 16 {
		end := min(start+16, len(data))
		chunk := data[start:end]
		_, _ = fmt.Fprintf(writer, "%08x  ", int(opts.skip)+start)
		for i := 0; i < 16; i++ {
			if i == 8 {
				_, _ = writer.WriteString(" ")
			}
			if i < len(chunk) {
				_, _ = fmt.Fprintf(writer, "%02x ", chunk[i])
			} else {
				_, _ = writer.WriteString("   ")
			}
		}
		_, _ = fmt.Fprintf(writer, " |%s|\n", printableBytes(chunk))
	}
	_, _ = fmt.Fprintf(writer, "%08x\n", int(opts.skip)+len(data))
	_ = writer.Flush()
	return 0
}

func printableBytes(data []byte) string {
	var output strings.Builder
	for _, value := range data {
		if value >= 0x20 && value <= 0x7e {
			output.WriteByte(value)
		} else {
			output.WriteByte('.')
		}
	}
	return output.String()
}
