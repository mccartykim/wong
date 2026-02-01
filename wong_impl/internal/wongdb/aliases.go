package wongdb

import (
	"context"
	"fmt"
	"strings"
)

// jjAliases defines convenience aliases installed into the jj repo config.
// These allow users to run `jj wong-list`, `jj wong-show`, etc.
var jjAliases = map[string][]string{
	"wong-list":    {"util", "exec", "--", "wong", "list"},
	"wong-show":    {"util", "exec", "--", "wong", "show"},
	"wong-create":  {"util", "exec", "--", "wong", "create"},
	"wong-close":   {"util", "exec", "--", "wong", "close"},
	"wong-comment": {"util", "exec", "--", "wong", "comment"},
	"wong-sync":    {"util", "exec", "--", "wong", "sync"},
}

// InstallAliases installs jj command aliases for wong into the repo config.
// After installation, users can run `jj wong-list` etc.
func (db *WongDB) InstallAliases(ctx context.Context) error {
	for name, cmd := range jjAliases {
		// Format as TOML array: ["util", "exec", "--", "wong", "list"]
		parts := make([]string, len(cmd))
		for i, c := range cmd {
			parts[i] = fmt.Sprintf("%q", c)
		}
		value := "[" + strings.Join(parts, ", ") + "]"

		configKey := fmt.Sprintf("aliases.%s", name)
		if _, err := db.runJJ(ctx, "config", "set", "--repo", configKey, value); err != nil {
			return fmt.Errorf("wongdb: failed to install alias %s: %w", name, err)
		}
	}
	return nil
}

// UninstallAliases removes wong jj command aliases from the repo config.
func (db *WongDB) UninstallAliases(ctx context.Context) error {
	for name := range jjAliases {
		configKey := fmt.Sprintf("aliases.%s", name)
		// Ignore errors - alias might not exist
		db.runJJ(ctx, "config", "unset", "--repo", configKey)
	}
	return nil
}

// HasAliases checks if wong jj aliases are installed.
func (db *WongDB) HasAliases(ctx context.Context) bool {
	// Check if at least one alias exists
	output, err := db.runJJ(ctx, "config", "get", "aliases.wong-list")
	return err == nil && strings.TrimSpace(output) != ""
}
