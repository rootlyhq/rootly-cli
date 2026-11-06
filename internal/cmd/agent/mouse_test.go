package agent

import "testing"

func TestAgentMouseDisabled(t *testing.T) {
	for _, tc := range []struct {
		flag      bool
		env, tmux string
		want      bool
	}{
		{false, "", "", false}, {true, "", "", true}, {false, "1", "", true},
		{false, "TRUE", "", true}, {false, " yes ", "", true}, {false, "false", "", false},
		{false, "invalid", "", false}, {false, "0", "/tmp/tmux", true},
	} {
		if got := agentMouseDisabled(tc.flag, tc.env, tc.tmux); got != tc.want {
			t.Errorf("%+v: got %v", tc, got)
		}
	}
}
