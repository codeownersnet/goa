package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
	"github.com/codeownersnet/goa/tool/truncate"
)

type Config struct {
	AllowedPaths   []string
	WorkDir        string
	Timeout        int
	MaxOutputBytes int64
}

type gitResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Toolset struct {
	tools []tool.Tool
}

func (ts *Toolset) Tools() []tool.Tool { return ts.tools }

var _ tool.Toolset = (*Toolset)(nil)

func defaultTimeout(cfg Config) int {
	if cfg.Timeout <= 0 {
		return 120
	}
	return cfg.Timeout
}

func defaultMaxOutput(cfg Config) int64 {
	if cfg.MaxOutputBytes <= 0 {
		return 1 << 20
	}
	return cfg.MaxOutputBytes
}

func resolveWorkDir(argsWorkDir string, cfg Config) string {
	if argsWorkDir != "" {
		return argsWorkDir
	}
	return cfg.WorkDir
}

func runGit(ctx context.Context, cfg Config, guard *pathguard.PathGuard, workDir string, args ...string) (gitResult, error) {
	if guard != nil && workDir != "" {
		if err := guard.Check(workDir); err != nil {
			return gitResult{}, err
		}
	}

	timeout := defaultTimeout(cfg)
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "git", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := runCommand(cmdCtx, cmd)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return gitResult{}, err
		}
	}

	maxBytes := defaultMaxOutput(cfg)
	outStr := stdout.String()
	errStr := stderr.String()

	if int64(len(outStr)) > maxBytes {
		outStr = truncate.TruncateString(outStr, int(maxBytes))
	}
	if int64(len(errStr)) > maxBytes {
		errStr = truncate.TruncateString(errStr, int(maxBytes))
	}

	return gitResult{
		Stdout:   outStr,
		Stderr:   errStr,
		ExitCode: exitCode,
	}, nil
}

func resultToMap(r gitResult) map[string]any {
	return map[string]any{
		"stdout":    r.Stdout,
		"stderr":    r.Stderr,
		"exit_code": r.ExitCode,
	}
}

func runCommand(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	select {
	case err := <-waitCh:
		if err != nil {
			return fmt.Errorf("git: command execution: %w", err)
		}
		return nil
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		err := <-waitCh
		if err != nil {
			return fmt.Errorf("%w: %w", ctx.Err(), err)
		}
		return ctx.Err()
	}
}

func AllTools(cfg Config) (*Toolset, error) {
	guard := pathguard.New(cfg.AllowedPaths)
	tools := make([]tool.Tool, 0, 11)

	if t, err := newCloneTool(cfg, guard); err != nil {
		return nil, err
	} else {
		tools = append(tools, t)
	}
	if t, err := newPullTool(cfg, guard); err != nil {
		return nil, err
	} else {
		tools = append(tools, t)
	}
	if t, err := newPushTool(cfg, guard); err != nil {
		return nil, err
	} else {
		tools = append(tools, t)
	}
	if t, err := newAddTool(cfg, guard); err != nil {
		return nil, err
	} else {
		tools = append(tools, t)
	}
	if t, err := newBranchTool(cfg, guard); err != nil {
		return nil, err
	} else {
		tools = append(tools, t)
	}
	if t, err := newCommitTool(cfg, guard); err != nil {
		return nil, err
	} else {
		tools = append(tools, t)
	}
	if t, err := newStashTool(cfg, guard); err != nil {
		return nil, err
	} else {
		tools = append(tools, t)
	}
	if t, err := newStatusTool(cfg, guard); err != nil {
		return nil, err
	} else {
		tools = append(tools, t)
	}
	if t, err := newDiffTool(cfg, guard); err != nil {
		return nil, err
	} else {
		tools = append(tools, t)
	}
	if t, err := newLogTool(cfg, guard); err != nil {
		return nil, err
	} else {
		tools = append(tools, t)
	}
	if t, err := newCheckoutTool(cfg, guard); err != nil {
		return nil, err
	} else {
		tools = append(tools, t)
	}

	return &Toolset{tools: tools}, nil
}

func NewCloneTool(cfg Config) (tool.Tool, error) {
	return newCloneTool(cfg, pathguard.New(cfg.AllowedPaths))
}

func NewPullTool(cfg Config) (tool.Tool, error) {
	return newPullTool(cfg, pathguard.New(cfg.AllowedPaths))
}

func NewPushTool(cfg Config) (tool.Tool, error) {
	return newPushTool(cfg, pathguard.New(cfg.AllowedPaths))
}

func NewAddTool(cfg Config) (tool.Tool, error) {
	return newAddTool(cfg, pathguard.New(cfg.AllowedPaths))
}

func NewBranchTool(cfg Config) (tool.Tool, error) {
	return newBranchTool(cfg, pathguard.New(cfg.AllowedPaths))
}

func NewCommitTool(cfg Config) (tool.Tool, error) {
	return newCommitTool(cfg, pathguard.New(cfg.AllowedPaths))
}

func NewStashTool(cfg Config) (tool.Tool, error) {
	return newStashTool(cfg, pathguard.New(cfg.AllowedPaths))
}

func NewStatusTool(cfg Config) (tool.Tool, error) {
	return newStatusTool(cfg, pathguard.New(cfg.AllowedPaths))
}

func NewDiffTool(cfg Config) (tool.Tool, error) {
	return newDiffTool(cfg, pathguard.New(cfg.AllowedPaths))
}

func NewLogTool(cfg Config) (tool.Tool, error) {
	return newLogTool(cfg, pathguard.New(cfg.AllowedPaths))
}

func NewCheckoutTool(cfg Config) (tool.Tool, error) {
	return newCheckoutTool(cfg, pathguard.New(cfg.AllowedPaths))
}
