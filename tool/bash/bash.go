package bash

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
	"github.com/codeownersnet/goa/tool/internal/pathguard"
	"github.com/codeownersnet/goa/tool/truncate"
)

type Config struct {
	AllowedCommands []string
	DeniedCommands  []string
	AllowedPaths    []string
	Timeout         int
	MaxOutputBytes  int64
	WorkDir         string
	Env             map[string]string
}

type bashArgs struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	WorkDir string   `json:"workdir,omitempty"`
	Timeout int      `json:"timeout,omitempty"`
}

type Tool struct {
	tool.Tool
}

var shellInvokers = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "fish": true, "dash": true, "ksh": true, "csh": true, "tcsh": true,
}

var shellBuiltins = map[string]bool{
	"cd": true, "echo": true, "pwd": true, "export": true, "source": true, "set": true,
	"exit": true, "true": true, "false": true, "test": true, "type": true, "command": true,
	"readonly": true, "local": true, "return": true, "break": true, "continue": true,
}

func extractCommands(cmd string) []string {
	var allCmds []string

	segments := splitShellChain(cmd)
	for _, segment := range segments {
		piped := extractPipedCommands(segment)
		allCmds = append(allCmds, piped...)
	}

	seen := make(map[string]bool)
	var unique []string
	for _, c := range allCmds {
		if !seen[c] {
			seen[c] = true
			unique = append(unique, c)
		}
	}
	return unique
}

func splitShellChain(cmd string) []string {
	var segments []string
	for _, part := range strings.Split(cmd, "&&") {
		for _, sub := range strings.Split(part, ";") {
			s := strings.TrimSpace(sub)
			if s != "" {
				segments = append(segments, s)
			}
		}
	}
	return segments
}

func extractPipedCommands(cmd string) []string {
	var commands []string
	parts := strings.Split(cmd, "|")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			first := strings.SplitN(part, " ", 2)[0]
			if first != "" {
				commands = append(commands, first)
			}
		}
	}
	return commands
}

func New(cfg Config) (*Tool, error) {
	defaultTimeout := cfg.Timeout
	if defaultTimeout <= 0 {
		defaultTimeout = 120
	}
	maxOutputBytes := cfg.MaxOutputBytes
	if maxOutputBytes <= 0 {
		maxOutputBytes = 1 << 20
	}
	guard := pathguard.New(cfg.AllowedPaths)

	ft, err := functiontool.New(functiontool.Config{
		Name:        "bash",
		Description: "Runs a shell command and returns its stdout, stderr, and exit code. Each invocation runs a single command. The command string is executed via sh -c, so pipes, redirects, and shell features work.",
	}, func(ctx context.Context, args bashArgs) (map[string]any, error) {
		allCommands := extractCommands(args.Command)
		pipedCommands := extractPipedCommands(args.Command)
		allCommands = append(allCommands, pipedCommands...)

		if len(cfg.DeniedCommands) > 0 {
			for _, cmd := range allCommands {
				for _, denied := range cfg.DeniedCommands {
					if cmd == denied {
						return nil, fmt.Errorf("bash: command %q is denied", cmd)
					}
				}
			}
		}

		if len(cfg.AllowedCommands) > 0 {
			for _, cmd := range allCommands {
				if shellInvokers[cmd] || shellBuiltins[cmd] {
					continue
				}
				allowed := false
				for _, a := range cfg.AllowedCommands {
					if cmd == a {
						allowed = true
						break
					}
				}
				if !allowed {
					return nil, fmt.Errorf("bash: command %q is not in the allowed list (allowed: %v)", cmd, cfg.AllowedCommands)
				}
			}
		}

		if guard != nil && args.WorkDir != "" {
			if err := guard.Check(args.WorkDir); err != nil {
				return nil, fmt.Errorf("bash: %w", err)
			}
		}

		timeout := args.Timeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}

		workDir := args.WorkDir
		if workDir == "" {
			workDir = cfg.WorkDir
		}

		cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()

		var cmd *exec.Cmd
		if len(args.Args) > 0 && !strings.Contains(args.Command, " ") {
			cmd = exec.Command(args.Command, args.Args...)
		} else {
			cmd = exec.Command("sh", "-c", args.Command)
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if workDir != "" {
			cmd.Dir = workDir
		}
		if len(cfg.Env) > 0 {
			for k, v := range cfg.Env {
				cmd.Env = append(cmd.Environ(), k+"="+v)
			}
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
				return nil, fmt.Errorf("bash: %w", err)
			}
		}

		outStr := stdout.String()
		errStr := stderr.String()

		if int64(len(outStr)) > maxOutputBytes {
			outStr = truncate.TruncateString(outStr, int(maxOutputBytes))
		}
		if int64(len(errStr)) > maxOutputBytes {
			errStr = truncate.TruncateString(errStr, int(maxOutputBytes))
		}

		return map[string]any{
			"stdout":    outStr,
			"stderr":    errStr,
			"exit_code": exitCode,
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return &Tool{Tool: ft}, nil
}

var _ tool.Tool = (*Tool)(nil)

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
			return fmt.Errorf("bash: command execution: %w", err)
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
