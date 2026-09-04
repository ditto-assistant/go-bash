package gobash

import (
	"sort"
	"strings"
)

// splitArgs separates short flag characters from positional operands using a
// simplified getopt model good enough for the common coreutils: tokens that
// start with "-" (but are not exactly "-" and not "--") contribute their
// individual characters as flags; "--" terminates flag parsing; everything else
// is an operand. It does not handle flags that take values.
func splitArgs(args []string) (flags map[rune]bool, operands []string) {
	flags = map[rune]bool{}
	endOfFlags := false
	for _, a := range args {
		switch {
		case endOfFlags:
			operands = append(operands, a)
		case a == "--":
			endOfFlags = true
		case len(a) > 1 && strings.HasPrefix(a, "-"):
			for _, c := range a[1:] {
				flags[c] = true
			}
		default:
			operands = append(operands, a)
		}
	}
	return flags, operands
}

// validateShortFlags rejects every flag the command does not explicitly
// implement. Silently accepting flags is particularly dangerous for mutating
// commands such as cp, mv, and rm because callers may rely on their safety
// semantics.
func validateShortFlags(e *Env, flags map[rune]bool, allowed string) bool {
	var unsupported []rune
	for flag := range flags {
		if !strings.ContainsRune(allowed, flag) {
			unsupported = append(unsupported, flag)
		}
	}
	if len(unsupported) == 0 {
		return true
	}
	sort.Slice(unsupported, func(i, j int) bool { return unsupported[i] < unsupported[j] })
	e.Errorf("unsupported option -- %c", unsupported[0])
	return false
}
