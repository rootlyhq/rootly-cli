package builtin

import (
	"testing"
)

func TestKubectlSecretAliasApprovalHeuristics(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "get pods", args: []string{"get", "pods"}, want: false},
		{name: "get services", args: []string{"get", "services"}, want: false},
		{name: "get svc", args: []string{"get", "svc"}, want: false},
		{name: "describe deployment", args: []string{"describe", "deploy/web"}, want: false},
		{name: "pod logs", args: []string{"logs", "pod/x"}, want: false},
		{name: "short secret alias", args: []string{"get", "se", "x", "-o", "yaml"}, want: true},
		{name: "uppercase short alias", args: []string{"get", "SE"}, want: true},
		{name: "comma-separated alias", args: []string{"get", "pods,se"}, want: true},
		{name: "slash-separated alias", args: []string{"get", "se/foo"}, want: true},
		{name: "plural group-qualified alias", args: []string{"describe", "secrets.v1", "x"}, want: true},
		{name: "singular group-qualified alias", args: []string{"describe", "secret.v1", "x"}, want: true},
		{name: "case-insensitive Secret", args: []string{"get", "Secret"}, want: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := cliNeedsApproval("kubectl", test.args); got != test.want {
				t.Fatalf("cliNeedsApproval(kubectl, %v) = %t, want %t", test.args, got, test.want)
			}
		})
	}
}
