package ansible

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// RolesPath returns the effective DEFAULT_ROLES_PATH, as resolved by
// `ansible-config dump` from the project directory.
func (c *Client) RolesPath(ctx context.Context) ([]string, error) {
	out, _, err := c.output(ctx, "ansible-config", "dump", "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseRolesPath(out)
}

// parseRolesPath reads the DEFAULT_ROLES_PATH entry of an
// `ansible-config dump --format json` document.
func parseRolesPath(data []byte) ([]string, error) {
	var entries []struct {
		Name  string          `json:"name"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse ansible-config dump: %w", err)
	}
	for _, e := range entries {
		if e.Name != "DEFAULT_ROLES_PATH" {
			continue
		}
		var paths []string
		if err := json.Unmarshal(e.Value, &paths); err != nil {
			return nil, fmt.Errorf("parse DEFAULT_ROLES_PATH: %w", err)
		}
		return paths, nil
	}
	return nil, errors.New("DEFAULT_ROLES_PATH not found in ansible-config dump")
}
