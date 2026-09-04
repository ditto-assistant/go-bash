package gobash

import (
	"context"
	"encoding/json"
	"fmt"
)

func init() { Register("gobash", cmdGobash) }

type shellInfo struct {
	Shell             string   `json:"shell"`
	BashVersion       string   `json:"bash_version"`
	BashCompatibility string   `json:"bash_compatibility"`
	Runtime           string   `json:"runtime"`
	RuntimeVersion    string   `json:"runtime_version"`
	Version           string   `json:"version"`
	Cwd               string   `json:"cwd"`
	HostAccess        bool     `json:"host_access"`
	NetworkAccess     bool     `json:"network_access"`
	FreshShellPerRun  bool     `json:"fresh_shell_per_run"`
	ExternalCommands  []string `json:"external_commands"`
	BashLimitations   []string `json:"bash_limitations"`
}

func cmdGobash(_ context.Context, e *Env) int {
	if len(e.Args) == 1 || e.Args[1] == "help" || e.Args[1] == "--help" || e.Args[1] == "-h" {
		_, err := fmt.Fprintln(e.Stdout, `go-bash: bounded Bash-compatible interpreter (mvdan/sh)

Usage:
  gobash info [--json]       show runtime and sandbox metadata
  gobash commands [--json]   list every available external command
  gobash --version           show the runtime identity

This targets GNU Bash 5.3 syntax and exposes a suffixed BASH_VERSION, but it is
not the GNU Bash executable. Shell builtins such as cd, command, echo, printf,
pwd, test, and read are
provided by mvdan/sh. External commands are pure-Go implementations over the
virtual filesystem. No host executables, host files, network, Python, Node.js,
package installation, zsh, or nested shells are available. The VFS persists
between host Run calls, while each call starts a fresh shell variable/function
and cwd state. Run 'gobash info --json' for known Bash compatibility limits.`)
		if err != nil {
			e.Errorf("%v", err)
			return 1
		}
		return 0
	}
	switch e.Args[1] {
	case "--version", "version":
		if _, err := fmt.Fprintf(e.Stdout, "go-bash %s (Bash %s compatible; %s)\n", Version, BashCompatibility, RuntimeVersion); err != nil {
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
			Shell: "bash", BashVersion: BashVersion, BashCompatibility: BashCompatibility,
			Runtime: Runtime, RuntimeVersion: RuntimeVersion, Version: Version, Cwd: e.Dir,
			FreshShellPerRun: true, ExternalCommands: Commands(), BashLimitations: BashLimitations(),
		}
		if len(e.Args) > 2 && e.Args[2] == "--json" {
			if err := json.NewEncoder(e.Stdout).Encode(info); err != nil {
				e.Errorf("%v", err)
				return 1
			}
			return 0
		}
		_, err := fmt.Fprintf(e.Stdout, "shell: %s\nbash version: %s\nbash compatibility: %s\nruntime: %s\nruntime version: %s\ngo-bash version: %s\ncwd: %s\nhost access: false\nnetwork access: false\nfresh shell per run: true\n", info.Shell, info.BashVersion, info.BashCompatibility, info.Runtime, info.RuntimeVersion, info.Version, info.Cwd)
		for _, limitation := range info.BashLimitations {
			_, err = fmt.Fprintf(e.Stdout, "bash limitation: %s\n", limitation)
			if err != nil {
				break
			}
		}
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
