package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/bittorrent/download"
	"github.com/example/bittorrent/metainfo"
)

func main() {
	torrentFile := flag.String("torrent", "", "Path to .torrent file")
	outputDir := flag.String("output", "./downloads", "Output directory for downloaded files")
	listenPort := flag.Int("port", 6881, "Port to listen for incoming peer connections")
	maxPeers := flag.Int("max-peers", 30, "Maximum concurrent peer connections")

	flag.Parse()

	if *torrentFile == "" {
		fmt.Fprintf(os.Stderr, "Error: -torrent flag is required\n")
		flag.Usage()
		os.Exit(1)
	}

	// Parse the torrent file
	torrentData, err := ioutil.ReadFile(*torrentFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading torrent file: %v\n", err)
		os.Exit(1)
	}

	meta, err := metainfo.ParseTorrent(torrentData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing torrent: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Torrent: %s\n", meta.Name)
	fmt.Printf("Total size: %d bytes\n", meta.TotalLength)
	fmt.Printf("Pieces: %d (%.0f MB each)\n", len(meta.Pieces), float64(meta.PieceLength)/1024/1024)
	fmt.Printf("Announce URL: %s\n", meta.Announce)
	fmt.Printf("Output directory: %s\n", *outputDir)
	fmt.Println()

	// Create the orchestrator
	cfg := &download.Config{
		Meta:       meta,
		OutputDir:  *outputDir,
		TrackerURL: meta.Announce,
		ListenPort: uint16(*listenPort),
		MaxPeers:   *maxPeers,
	}

	orch, err := download.NewOrchestrator(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating orchestrator: %v\n", err)
		os.Exit(1)
	}

	// Set up signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start the download
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := orch.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting download: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Download started. Press Ctrl+C to stop.")

	// Wait for either completion or signal
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Wait for the download to complete
	if err := orch.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "Error during download: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nDownload completed successfully!")
}
