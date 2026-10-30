package flow

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/truncate"
)

const defaultResultMaxBytes = 50 * 1024

type ResultPolicy interface {
	Process(ctx tool.Context, result map[string]any) (map[string]any, error)
}

type ResultPolicyFunc func(tool.Context, map[string]any) (map[string]any, error)

func (f ResultPolicyFunc) Process(ctx tool.Context, result map[string]any) (map[string]any, error) {
	return f(ctx, result)
}

type TruncatePolicyConfig struct {
	MaxBytes    int
	SpillDir    string
	SpillMaxAge time.Duration
}

type truncateResultPolicy struct {
	cfg TruncatePolicyConfig
}

func NewTruncateResultPolicy(cfg TruncatePolicyConfig) ResultPolicy {
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = defaultResultMaxBytes
	}
	if cfg.SpillMaxAge <= 0 {
		cfg.SpillMaxAge = 7 * 24 * time.Hour
	}
	return &truncateResultPolicy{cfg: cfg}
}

var defaultResultPolicy = NewTruncateResultPolicy(TruncatePolicyConfig{})

func (p *truncateResultPolicy) Process(ctx tool.Context, result map[string]any) (map[string]any, error) {
	if result == nil {
		return nil, nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal tool result: %w", err)
	}
	if len(data) <= p.cfg.MaxBytes {
		return result, nil
	}

	spillPath := ""
	if p.cfg.SpillDir != "" {
		if path, spillErr := p.spill(ctx, data); spillErr == nil {
			spillPath = path
		}
	}

	perString := p.cfg.MaxBytes
	for {
		candidate := truncateResultStrings(result, perString)
		candidate["_output_truncated"] = true
		if spillPath != "" {
			candidate["_output_path"] = spillPath
		}

		encoded, err := json.Marshal(candidate)
		if err != nil {
			return nil, fmt.Errorf("marshal truncated tool result: %w", err)
		}
		if len(encoded) <= p.cfg.MaxBytes {
			return candidate, nil
		}
		if perString <= 64 {
			return boundedResultPreview(data, p.cfg.MaxBytes, spillPath), nil
		}
		perString /= 2
	}
}

func (p *truncateResultPolicy) spill(ctx tool.Context, data []byte) (string, error) {
	runDir := filepath.Join(p.cfg.SpillDir, resultRunDir(ctx))
	cleanupResultRunDirs(p.cfg.SpillDir, p.cfg.SpillMaxAge)

	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return "", fmt.Errorf("create result spill directory: %w", err)
	}
	path := filepath.Join(runDir, fmt.Sprintf("tool_%d.json", time.Now().UnixNano()))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write result spill file: %w", err)
	}
	return path, nil
}

func resultRunDir(ctx tool.Context) string {
	id := ""
	if ctx != nil {
		id = ctx.SessionID() + "\x00" + ctx.InvocationID()
	}
	sum := sha256.Sum256([]byte(id))
	return fmt.Sprintf("%x", sum[:8])
}

func cleanupResultRunDirs(root string, maxAge time.Duration) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if !entry.IsDir() || !isResultRunDir(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
}

func isResultRunDir(name string) bool {
	if len(name) != 16 {
		return false
	}
	for _, r := range name {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func truncateResultStrings(value any, maxLen int) map[string]any {
	truncated, ok := truncateValue(value, maxLen).(map[string]any)
	if !ok {
		return make(map[string]any)
	}
	return truncated
}

func truncateValue(value any, maxLen int) any {
	switch v := value.(type) {
	case string:
		return truncate.TruncateString(v, maxLen)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			out[k] = truncateValue(child, maxLen)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = truncateValue(child, maxLen)
		}
		return out
	default:
		return v
	}
}

func boundedResultPreview(data []byte, maxBytes int, spillPath string) map[string]any {
	budget := maxBytes
	for budget > 0 {
		candidate := map[string]any{
			"_output_truncated": true,
			"preview":           truncate.TruncateString(string(data), budget),
		}
		if spillPath != "" {
			candidate["_output_path"] = spillPath
		}

		encoded, err := json.Marshal(candidate)
		if err == nil && len(encoded) <= maxBytes {
			return candidate
		}
		budget /= 2
	}

	result := map[string]any{"_output_truncated": true}
	if spillPath != "" {
		result["_output_path"] = spillPath
	}
	return result
}
