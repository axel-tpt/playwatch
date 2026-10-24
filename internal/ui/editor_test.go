package ui

import (
	"slices"
	"testing"
)

func TestEditorCommand(t *testing.T) {
	cmd, err := editorCommand("code -w", "/p")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"code", "-w", "/p"}; !slices.Equal(cmd.Args, want) {
		t.Errorf("args = %v, want %v", cmd.Args, want)
	}
	if _, err := editorCommand("  ", "/p"); err == nil {
		t.Error("blank editor: want error")
	}
}

func TestEditorUnset(t *testing.T) {
	t.Setenv("EDITOR", "")
	msg := Edit("/p")()
	st, ok := msg.(statusMsg)
	if !ok || !st.err {
		t.Fatalf("got %#v, want error statusMsg", msg)
	}
}
