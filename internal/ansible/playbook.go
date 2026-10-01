package ansible

import (
	"context"
	"fmt"
)

// PlaybookOutline returns the output of `ansible-playbook --list-hosts
// --list-tasks`, which describes what a run would target without running
// anything. It is meant to be displayed as is.
func (c *Client) PlaybookOutline(ctx context.Context, playbook string) (string, error) {
	args := append(c.inventoryArgs(), "--list-hosts", "--list-tasks", playbook)
	out, _, err := c.output(ctx, "ansible-playbook", args...)
	if err != nil {
		return "", fmt.Errorf("outline of %s: %w", playbook, err)
	}
	return string(out), nil
}
