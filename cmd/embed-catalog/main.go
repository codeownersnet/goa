package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const defaultURL = "https://models.dev/api.json"

func main() {
	url := flag.String("url", defaultURL, "source URL of the models.dev catalog")
	out := flag.String("out", defaultOutPath(), "output path (defaults to modelsdev/catalog.json at the repo root)")
	flag.Parse()

	if err := run(context.Background(), *url, *out); err != nil {
		fmt.Fprintf(os.Stderr, "embed-catalog: %v\n", err)
		os.Exit(1)
	}
}

func defaultOutPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("modelsdev", "catalog.json")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "modelsdev", "catalog.json")
}

func run(ctx context.Context, url, out string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch catalog: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read catalog response: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if err := os.WriteFile(out, body, 0o644); err != nil {
		return fmt.Errorf("write catalog: %w", err)
	}

	fmt.Printf("wrote %d bytes to %s\n", len(body), out)
	return nil
}
