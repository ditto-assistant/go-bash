package gobash

import (
	"encoding/json"
	"testing"
)

func TestGobashCapabilityInventory(t *testing.T) {
	result := run(t, New(), `gobash info --json`)
	if result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("gobash info: %+v", result)
	}
	var info shellInfo
	if err := json.Unmarshal([]byte(result.Stdout), &info); err != nil {
		t.Fatalf("decode info: %v", err)
	}
	if info.Shell != "bash" || info.Runtime != "go-bash/mvdan-sh" || info.RuntimeVersion == "" ||
		info.BashVersion == "" || info.BashCompatibility != "5.3" || info.Cwd != "/tmp" || info.HostAccess || info.NetworkAccess {
		t.Fatalf("unexpected info: %+v", info)
	}
	if len(info.ExternalCommands) != len(Commands()) {
		t.Fatalf("inventory has %d commands, registry has %d", len(info.ExternalCommands), len(Commands()))
	}
}

func TestBashCompatibilityVersionVariables(t *testing.T) {
	result := run(t, New(), `printf '%s|%s\n' "$BASH_VERSION" "$BASH_COMPAT"`)
	if result.ExitCode != 0 || result.Stdout != BashVersion+"|"+BashCompatibility+"\n" || result.Stderr != "" {
		t.Fatalf("bash compatibility variables: %+v", result)
	}
}
