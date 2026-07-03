package listdir

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
)

type Config struct {
	AllowedPaths []string
}

type listDirArgs struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

type dirEntry struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

func New(cfg Config) (tool.Tool, error) {
	guard := pathguard.New(cfg.AllowedPaths)

	return functiontool.New(functiontool.Config{
		Name:        "list_dir",
		Description: "Lists the contents of a directory. Returns entry names, types (file/dir), sizes, and modification times. Optionally lists recursively.",
	}, func(_ context.Context, args listDirArgs) (map[string]any, error) {
		if err := guard.Check(args.Path); err != nil {
			return nil, fmt.Errorf("list_dir: %w", err)
		}

		entries := make([]dirEntry, 0)
		var err error

		if args.Recursive {
			err = filepath.WalkDir(args.Path, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				info, infoErr := d.Info()
				if infoErr != nil {
					return infoErr
				}
				entryType := "file"
				if d.IsDir() {
					entryType = "dir"
				}
				entries = append(entries, dirEntry{
					Name:    path,
					Type:    entryType,
					Size:    info.Size(),
					ModTime: info.ModTime().Format("2006-01-02T15:04:05"),
				})
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("list_dir: %w", err)
			}
		} else {
			entries, err = readDir(args.Path)
			if err != nil {
				return nil, fmt.Errorf("list_dir: %w", err)
			}
		}

		return map[string]any{"entries": entries}, nil
	})
}

func readDir(path string) ([]dirEntry, error) {
	infos, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	entries := make([]dirEntry, 0, len(infos))
	for _, d := range infos {
		info, err := d.Info()
		if err != nil {
			continue
		}
		entryType := "file"
		if d.IsDir() {
			entryType = "dir"
		}
		entries = append(entries, dirEntry{
			Name:    d.Name(),
			Type:    entryType,
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02T15:04:05"),
		})
	}
	return entries, nil
}
