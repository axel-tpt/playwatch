// Command playwatch is a k9s-style terminal UI for Ansible.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Ansible runs in its own process group: ctrl+c reaches it through the
	// context cancellation, which interrupts it gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := newRootCmd().ExecuteContext(ctx)
	stop()

	var exitErr exitCodeError
	switch {
	case errors.As(err, &exitErr):
		os.Exit(exitErr.code)
	case err != nil:
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
