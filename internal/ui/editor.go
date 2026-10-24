package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// editorCommand builds the command opening path in editor. The editor is
// split on whitespace and run without a shell.
func editorCommand(editor, path string) (*exec.Cmd, error) {
	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return nil, errors.New("$EDITOR is not set")
	}
	return exec.Command(fields[0], append(fields[1:], path)...), nil
}

// Edit opens path in $EDITOR, suspending the terminal until the editor exits,
// then refreshes the current list (or shows the error).
func Edit(path string) tea.Cmd {
	cmd, err := editorCommand(os.Getenv("EDITOR"), path)
	if err != nil {
		return Error(err)
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return statusMsg{text: fmt.Sprintf("editor: %v", err), err: true}
		}
		return refreshMsg{}
	})
}
