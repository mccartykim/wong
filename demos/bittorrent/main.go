package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mccartykim/wong-bittorrent/download"
	"github.com/mccartykim/wong-bittorrent/metainfo"
)

// CLIConfig represents parsed command-line configuration
type CLIConfig struct {
	TorrentPath string
	OutputDir   string
	Port        int
	MaxPeers    int
}

// parseFlags parses command-line flags and returns the configuration.
// Returns an error if required flags are missing.
func parseFlags(args []string) (*CLIConfig, error) {
	fs := flag.NewFlagSet("bittorrent", flag.ContinueOnError)
	torrentPath := fs.String("torrent", "", "Path to .torrent file (required)")
	outputDir := fs.String("output", ".", "Output directory")
	port := fs.Int("port", 6881, "Listen port for incoming connections")
	maxPeers := fs.Int("max-peers", 30, "Maximum concurrent peer connections")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if *torrentPath == "" {
		return nil, fmt.Errorf("missing required flag: -torrent")
	}

	return &CLIConfig{
		TorrentPath: *torrentPath,
		OutputDir:   *outputDir,
		Port:        *port,
		MaxPeers:    *maxPeers,
	}, nil
}

func main() {
	cliCfg, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Usage: bittorrent -torrent <file.torrent> [-output dir] [-port N] [-max-peers N]\n")
		if err.Error() != "missing required flag: -torrent" {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}

	// Parse torrent file
	torrent, err := metainfo.ParseFromFile(cliCfg.TorrentPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing torrent file: %v\n", err)
		os.Exit(1)
	}

	// Print torrent info
	fmt.Printf("Torrent: %s\n", torrent.Name)
	fmt.Printf("Size: %d bytes (%.2f MB)\n", torrent.TotalLength(), float64(torrent.TotalLength())/(1024*1024))
	fmt.Printf("Pieces: %d\n", len(torrent.Pieces))
	fmt.Printf("Piece Length: %d bytes\n", torrent.PieceLength)
	fmt.Printf("Announce: %s\n", torrent.Announce)

	// Create download config
	cfg := &download.Config{
		Torrent:   torrent,
		OutputDir: cliCfg.OutputDir,
		Port:      cliCfg.Port,
		MaxPeers:  cliCfg.MaxPeers,
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Create downloader and run
	downloader := download.New(cfg)
	if err := downloader.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Download error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Download complete!")
}
