package main

import (
	"testing"
)

func TestParseFlags_MissingTorrentFlag(t *testing.T) {
	// Test that missing -torrent flag returns an error
	cfg, err := parseFlags([]string{})
	if err == nil {
		t.Fatal("expected error for missing -torrent flag, got nil")
	}
	if cfg != nil {
		t.Fatal("expected nil config when error occurs")
	}
	if err.Error() != "missing required flag: -torrent" {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestParseFlags_ValidFlags(t *testing.T) {
	// Test that valid flags are parsed correctly
	args := []string{
		"-torrent", "test.torrent",
		"-output", "/tmp/downloads",
		"-port", "9999",
		"-max-peers", "50",
	}
	cfg, err := parseFlags(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}

	if cfg.TorrentPath != "test.torrent" {
		t.Errorf("expected TorrentPath='test.torrent', got '%s'", cfg.TorrentPath)
	}
	if cfg.OutputDir != "/tmp/downloads" {
		t.Errorf("expected OutputDir='/tmp/downloads', got '%s'", cfg.OutputDir)
	}
	if cfg.Port != 9999 {
		t.Errorf("expected Port=9999, got %d", cfg.Port)
	}
	if cfg.MaxPeers != 50 {
		t.Errorf("expected MaxPeers=50, got %d", cfg.MaxPeers)
	}
}

func TestParseFlags_DefaultValues(t *testing.T) {
	// Test that default values are used when flags are not provided
	args := []string{"-torrent", "test.torrent"}
	cfg, err := parseFlags(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}

	if cfg.TorrentPath != "test.torrent" {
		t.Errorf("expected TorrentPath='test.torrent', got '%s'", cfg.TorrentPath)
	}
	if cfg.OutputDir != "." {
		t.Errorf("expected OutputDir='.', got '%s'", cfg.OutputDir)
	}
	if cfg.Port != 6881 {
		t.Errorf("expected Port=6881, got %d", cfg.Port)
	}
	if cfg.MaxPeers != 30 {
		t.Errorf("expected MaxPeers=30, got %d", cfg.MaxPeers)
	}
}

func TestParseFlags_OnlyTorrentRequired(t *testing.T) {
	// Test that only -torrent flag is required
	args := []string{"-torrent", "my.torrent"}
	cfg, err := parseFlags(args)
	if err != nil {
		t.Fatalf("unexpected error with only -torrent flag: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.TorrentPath != "my.torrent" {
		t.Errorf("expected TorrentPath='my.torrent', got '%s'", cfg.TorrentPath)
	}
}
