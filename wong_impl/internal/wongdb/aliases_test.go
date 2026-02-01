package wongdb

import (
	"context"
	"strings"
	"testing"
)

func TestWongDB_InstallAliases(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	// Init first
	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Install aliases
	if err := db.InstallAliases(ctx); err != nil {
		t.Fatalf("InstallAliases: %v", err)
	}

	// Verify at least one alias is installed
	if !db.HasAliases(ctx) {
		t.Error("HasAliases returned false after InstallAliases")
	}

	// Verify specific alias via jj config get
	out := runJJ(t, dir, "config", "get", "aliases.wong-list")
	if !strings.Contains(out, "wong") {
		t.Errorf("wong-list alias doesn't contain 'wong': %s", out)
	}
}

func TestWongDB_UninstallAliases(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Install then uninstall
	if err := db.InstallAliases(ctx); err != nil {
		t.Fatalf("InstallAliases: %v", err)
	}
	if err := db.UninstallAliases(ctx); err != nil {
		t.Fatalf("UninstallAliases: %v", err)
	}

	if db.HasAliases(ctx) {
		t.Error("HasAliases returned true after UninstallAliases")
	}
}

func TestWongDB_HasAliases_BeforeInstall(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if db.HasAliases(ctx) {
		t.Error("HasAliases returned true before InstallAliases")
	}
}
