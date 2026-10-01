// Package callback embeds the playwatch_events Ansible callback plugin.
package callback

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// Name is the plugin name to list in ANSIBLE_CALLBACKS_ENABLED.
const Name = "playwatch_events"

//go:embed playwatch_events.py
var plugin []byte

// Install writes the plugin to a new temporary directory, suitable for
// ANSIBLE_CALLBACK_PLUGINS. The caller must call cleanup once the run is over.
func Install() (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "playwatch-callback-")
	if err != nil {
		return "", nil, fmt.Errorf("create callback dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	if err := os.WriteFile(filepath.Join(dir, Name+".py"), plugin, 0o600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write callback plugin: %w", err)
	}
	return dir, cleanup, nil
}
