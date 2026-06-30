package gobash

import "strings"

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
