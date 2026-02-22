package protocol

import "testing"

func TestToolAllowed(t *testing.T) {
	t.Run("no lists allows all", func(t *testing.T) {
		spec := AgentSpec{}
		for _, name := range []string{"read_file", "exec", "create_ticket", "anything"} {
			if !spec.ToolAllowed(name) {
				t.Errorf("expected %q to be allowed with no lists", name)
			}
		}
	})

	t.Run("whitelist only allows listed", func(t *testing.T) {
		spec := AgentSpec{
			ToolsWhitelist: []string{"read_file", "write_file"},
		}
		if !spec.ToolAllowed("read_file") {
			t.Error("expected read_file to be allowed")
		}
		if !spec.ToolAllowed("write_file") {
			t.Error("expected write_file to be allowed")
		}
		if spec.ToolAllowed("exec") {
			t.Error("expected exec to be denied")
		}
		if spec.ToolAllowed("create_ticket") {
			t.Error("expected create_ticket to be denied")
		}
	})

	t.Run("blacklist blocks listed", func(t *testing.T) {
		spec := AgentSpec{
			ToolsBlacklist: []string{"exec", "create_ticket"},
		}
		if !spec.ToolAllowed("read_file") {
			t.Error("expected read_file to be allowed")
		}
		if spec.ToolAllowed("exec") {
			t.Error("expected exec to be blocked")
		}
		if spec.ToolAllowed("create_ticket") {
			t.Error("expected create_ticket to be blocked")
		}
	})

	t.Run("whitelist takes precedence over blacklist", func(t *testing.T) {
		spec := AgentSpec{
			ToolsWhitelist: []string{"read_file"},
			ToolsBlacklist: []string{"write_file"},
		}
		if !spec.ToolAllowed("read_file") {
			t.Error("expected read_file to be allowed (whitelisted)")
		}
		// write_file is blacklisted but whitelist takes precedence,
		// so only whitelisted items are allowed
		if spec.ToolAllowed("write_file") {
			t.Error("expected write_file to be denied (not in whitelist)")
		}
		if spec.ToolAllowed("exec") {
			t.Error("expected exec to be denied (not in whitelist)")
		}
	})
}
