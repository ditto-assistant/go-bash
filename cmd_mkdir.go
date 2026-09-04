package gobash

import (
	"context"
	"io/fs"
	"strconv"
	"strings"
)

func init() { Register("mkdir", cmdMkdir) }

// cmdMkdir creates directories. The -p flag creates parents as needed and does
// not error if the target already exists.
func cmdMkdir(_ context.Context, e *Env) int {
	parents, mode, operands, ok := parseMkdirArgs(e)
	if !ok {
		return 2
	}
	if len(operands) == 0 {
		e.Errorf("missing operand")
		return 1
	}
	code := 0
	for _, d := range operands {
		full := e.Resolve(d)
		var err error
		if parents {
			err = e.FS.MkdirAll(full, mode)
		} else {
			err = e.FS.Mkdir(full, mode)
		}
		if err != nil {
			e.Errorf("cannot create directory '%s': %v", d, err)
			code = 1
		}
	}
	return code
}

func parseMkdirArgs(e *Env) (parents bool, mode fs.FileMode, operands []string, ok bool) {
	mode = 0o755
	args := e.Args[1:]
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--":
			return parents, mode, append(operands, args[i+1:]...), true
		case args[i] == "-p" || args[i] == "--parents":
			parents = true
		case args[i] == "-m" || args[i] == "--mode":
			if i+1 >= len(args) {
				e.Errorf("option %s requires an argument", args[i])
				return false, 0, nil, false
			}
			i++
			parsed, err := strconv.ParseUint(args[i], 8, 9)
			if err != nil {
				e.Errorf("invalid mode: %s", args[i])
				return false, 0, nil, false
			}
			mode = fs.FileMode(parsed)
		case strings.HasPrefix(args[i], "-m") && len(args[i]) > 2:
			parsed, err := strconv.ParseUint(args[i][2:], 8, 9)
			if err != nil {
				e.Errorf("invalid mode: %s", args[i][2:])
				return false, 0, nil, false
			}
			mode = fs.FileMode(parsed)
		case strings.HasPrefix(args[i], "-"):
			e.Errorf("unsupported option: %s", args[i])
			return false, 0, nil, false
		default:
			operands = append(operands, args[i])
		}
	}
	return parents, mode, operands, true
}
