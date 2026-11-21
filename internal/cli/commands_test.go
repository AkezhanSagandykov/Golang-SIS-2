package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"taskflow/internal/service"
	"taskflow/internal/storage"
)

func newTestCLI(t *testing.T) (*CLI, *bytes.Buffer) {
	t.Helper()
	svc, err := service.New(storage.NewMemoryStore(), nil, nil, service.Config{Workers: 2, QueueSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Shutdown)
	out := &bytes.Buffer{}
	return New(svc, out, nil), out
}

// Help and dispatch come from one table: every command name is in help and is dispatched.
func TestCommandTable_HelpAndDispatchAgree(t *testing.T) {
	c, out := newTestCLI(t)
	if err := c.Execute("help"); err != nil {
		t.Fatal(err)
	}
	help := out.String()

	seen := map[string]bool{}
	for _, cmd := range commandTable() {
		if cmd.run == nil || cmd.usage == "" || cmd.summary == "" || len(cmd.names) == 0 {
			t.Fatalf("incomplete command entry: %+v", cmd)
		}
		for _, name := range cmd.names {
			if seen[name] {
				t.Errorf("command name %q is defined twice", name)
			}
			seen[name] = true
			if !strings.Contains(help, name) {
				t.Errorf("help does not mention %q", name)
			}
			if _, ok := c.byName[name]; !ok {
				t.Errorf("%q is in the table but not dispatched", name)
			}
		}
	}
}

func TestExecute_ExitAndQuitReturnErrExit(t *testing.T) {
	c, _ := newTestCLI(t)
	for _, line := range []string{"exit", "quit", "EXIT"} {
		if err := c.Execute(line); !errors.Is(err, ErrExit) {
			t.Errorf("Execute(%q) = %v, want ErrExit", line, err)
		}
	}
	if err := c.Execute("unknown"); err == nil || errors.Is(err, ErrExit) {
		t.Errorf("Execute(unknown) = %v, want an unknown-command error", err)
	}
}
