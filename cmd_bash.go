package gobash

import "context"

func init() { Register("bash", cmdBash) }

// cmdBash exposes runtime help and identity under the familiar shell name. It
// does not start a nested interpreter; scripts are already running in go-bash.
func cmdBash(ctx context.Context, e *Env) int {
	if len(e.Args) > 1 && e.Args[1] != "--help" && e.Args[1] != "-h" && e.Args[1] != "--version" {
		e.Errorf("nested shells are unavailable; the current interpreter is already Bash-compatible")
		return 2
	}
	return cmdGobash(ctx, e)
}
