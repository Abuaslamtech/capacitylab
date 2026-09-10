package cli

import (
	"bytes"
	"testing"
)

func TestRootCommandsRegistration(t *testing.T) {
	commands := rootCmd.Commands()

	expectedCommands := map[string]bool{
		"init":      false,
		"validate":  false,
		"run":       false,
		"history":   false,
		"compare":   false,
		"dashboard": false,
		"report":    false,
		"version":   false,
	}

	for _, cmd := range commands {
		if _, ok := expectedCommands[cmd.Name()]; ok {
			expectedCommands[cmd.Name()] = true
		}
	}

	for name, found := range expectedCommands {
		if !found {
			t.Errorf("Expected CLI command '%s' to be registered on rootCmd", name)
		}
	}
}

func TestCommandHelpOutputs(t *testing.T) {
	tests := []struct {
		cmdName string
	}{
		{"report"},
		{"dashboard"},
		{"run"},
		{"validate"},
		{"init"},
		{"compare"},
		{"history"},
	}

	for _, tc := range tests {
		t.Run(tc.cmdName+" help", func(t *testing.T) {
			cmd, _, err := rootCmd.Find([]string{tc.cmdName})
			if err != nil || cmd == nil {
				t.Fatalf("Could not find command %s: %v", tc.cmdName, err)
			}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			err = cmd.Help()
			if err != nil {
				t.Fatalf("Failed to print help for %s: %v", tc.cmdName, err)
			}
			if buf.Len() == 0 {
				t.Errorf("Expected help output for %s", tc.cmdName)
			}
		})
	}
}

func TestRunCommandFlags(t *testing.T) {
	flags := []string{"engine", "cpu", "memory", "cpuset", "cleanup", "json", "output-json", "json-output"}
	for _, f := range flags {
		if runCmd.Flags().Lookup(f) == nil {
			t.Errorf("Expected flag --%s to be defined on runCmd", f)
		}
	}
}
