package gobash

import (
	"context"
	"fmt"
	"strings"
)

func init() { Register("whoami", cmdWhoami) }

func cmdWhoami(_ context.Context, e *Env) int {
	user := "agent"
	for _, pair := range e.Environ {
		if value, ok := strings.CutPrefix(pair, "USER="); ok && value != "" {
			user = value
			break
		}
	}
	if _, err := fmt.Fprintln(e.Stdout, user); err != nil {
		e.Errorf("%v", err)
		return 1
	}
	return 0
}
