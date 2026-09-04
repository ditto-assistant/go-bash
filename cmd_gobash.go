package gobash

import (
	"context"
	"encoding/json"
	"fmt"
)

func init() { Register("gobash", cmdGobash) }

type shellInfo struct {
	Shell            string   `json:"shell"`
	Runtime          string   `json:"runtime"`
	Cwd              string   `json:"cwd"`
	HostAccess       bool     `json:"host_access"`
	NetworkAccess    bool     `json:"network_access"`
	ExternalCommands []string `json:"external_commands"`
}

func cmdGobash(_ context.Context, e *Env) int {
	if len(e.Args) == 1 || e.Args[1] == "help" || e.Args[1] == "--help" || e.Args[1] == "-h" {
		_, err := fmt.Fprintln(e.Stdout, `go-bash: bounded Bash-compatible interpreter (mvdan/sh)

Usage:
  gobash info [--json]       show runtime and sandbox metadata
  gobash commands [--json]   list every available external command
  gobash --version           show the runtime identity

Shell builtins such as cd, command, echo, printf, pwd, test, and read are
provided by mvdan/sh. External commands are pure-Go implementations over the
virtual filesystem. No host executables, host files, network, Python, Node.js,
package installation, or zsh are available.`)
		if err != nil {
			e.Errorf("%v", err)
			return 1
		}
		return 0
	}
	switch e.Args[1] {
	case "--version", "version":
		if _, err := fmt.Fprintln(e.Stdout, "go-bash (Bash-compatible; mvdan/sh)"); err != nil {
			e.Errorf("%v", err)
			return 1
		}
		return 0
	case "commands":
		commands := Commands()
		if len(e.Args) > 2 && e.Args[2] == "--json" {
			if err := json.NewEncoder(e.Stdout).Encode(commands); err != nil {
				e.Errorf("%v", err)
				return 1
			}
			return 0
		}
		for _, command := range commands {
			if _, err := fmt.Fprintln(e.Stdout, command); err != nil {
				e.Errorf("%v", err)
				return 1
			}
		}
		return 0
	case "info":
		info := shellInfo{
			Shell: "bash", Runtime: "go-bash/mvdan-sh", Cwd: e.Dir,
			ExternalCommands: Commands(),
		}
		if len(e.Args) > 2 && e.Args[2] == "--json" {
			if err := json.NewEncoder(e.Stdout).Encode(info); err != nil {
				e.Errorf("%v", err)
				return 1
			}
			return 0
		}
		_, err := fmt.Fprintf(e.Stdout, "shell: %s\nruntime: %s\ncwd: %s\nhost access: false\nnetwork access: false\n", info.Shell, info.Runtime, info.Cwd)
		if err != nil {
			e.Errorf("%v", err)
			return 1
		}
		return 0
	default:
		e.Errorf("unknown capability command %q; try 'gobash --help'", e.Args[1])
		return 2
	}
}
