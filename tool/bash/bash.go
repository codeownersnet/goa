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

const maxParseDepth = 8

func extractCommands(cmd string) ([]string, error) {
	allCmds, err := parseCommands(cmd, 0)
	if err != nil {
		return nil, err
	}
	return dedup(allCmds), nil
}

func dedup(cmds []string) []string {
	seen := make(map[string]bool)
	var unique []string
	for _, c := range cmds {
		if c != "" && !seen[c] {
			seen[c] = true
			unique = append(unique, c)
		}
	}
	return unique
}

func parseCommands(s string, depth int) ([]string, error) {
	if depth > maxParseDepth {
		return nil, fmt.Errorf("shell construct nesting exceeds depth %d", maxParseDepth)
	}
	var cmds []string
	var cur strings.Builder

	flush := func() {
		if fields := strings.Fields(cur.String()); len(fields) > 0 {
			cmds = append(cmds, fields[0])
		}
		cur.Reset()
	}

	inSingle, inDouble := false, false
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
			i++
		case inDouble:
			switch {
			case c == '"':
				inDouble = false
				i++
			case c == '\\':
				i += 2
			case c == '`':
				inner, next, err := extractBacktick(s, i)
				if err != nil {
					return nil, err
				}
				sub, err := parseCommands(inner, depth+1)
				if err != nil {
					return nil, err
				}
				cmds = append(cmds, sub...)
				i = next
			case c == '$' && i+1 < len(s) && s[i+1] == '(':
				inner, next, err := extractSubstitution(s, i)
				if err != nil {
					return nil, err
				}
				sub, err := parseCommands(inner, depth+1)
				if err != nil {
					return nil, err
				}
				cmds = append(cmds, sub...)
				i = next
			default:
				i++
			}
		default:
			switch {
			case c == '\'':
				inSingle = true
				i++
			case c == '"':
				inDouble = true
				i++
			case c == '\\':
				i += 2
			case c == '`':
				inner, next, err := extractBacktick(s, i)
				if err != nil {
					return nil, err
				}
				sub, err := parseCommands(inner, depth+1)
				if err != nil {
					return nil, err
				}
				cmds = append(cmds, sub...)
				i = next
			case c == '$' && i+1 < len(s) && s[i+1] == '(':
				inner, next, err := extractSubstitution(s, i)
				if err != nil {
					return nil, err
				}
				sub, err := parseCommands(inner, depth+1)
				if err != nil {
					return nil, err
				}
				cmds = append(cmds, sub...)
				i = next
			case c == ';' || c == '\n' || c == '\r':
				flush()
				i++
			case c == '&':
				flush()
				if i+1 < len(s) && s[i+1] == '&' {
					i += 2
				} else {
					i++
				}
			case c == '|':
				flush()
				if i+1 < len(s) && s[i+1] == '|' {
					i += 2
				} else {
					i++
				}
			default:
				cur.WriteByte(c)
				i++
			}
		}
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unbalanced quote")
	}
	flush()
	return cmds, nil
}

func extractBacktick(s string, i int) (string, int, error) {
	end := strings.IndexByte(s[i+1:], '`')
	if end < 0 {
		return "", 0, fmt.Errorf("unbalanced backtick")
	}
	return s[i+1 : i+1+end], i + end + 2, nil
}

func extractSubstitution(s string, i int) (string, int, error) {
	depth := 1
	for j := i + 2; j < len(s); j++ {
		switch s[j] {
		case '\'':
			end := strings.IndexByte(s[j+1:], '\'')
			if end < 0 {
				return "", 0, fmt.Errorf("unbalanced quote")
			}
			j += end + 1
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[i+2 : j], j + 1, nil
			}
		}
	}
	return "", 0, fmt.Errorf("unbalanced $() substitution")
}

func naiveExtractCommands(cmd string) []string {
	var cmds []string
	for _, part := range strings.Split(cmd, "&&") {
		for _, sub := range strings.Split(part, ";") {
			for _, piped := range strings.Split(sub, "|") {
				piped = strings.TrimSpace(piped)
				if fields := strings.Fields(piped); len(fields) > 0 {
					cmds = append(cmds, fields[0])
				}
			}
		}
	}
	return dedup(cmds)
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
	var guard *pathguard.PathGuard
	if len(cfg.AllowedPaths) > 0 {
		guard = pathguard.New(cfg.AllowedPaths)
	}

	ft, err := functiontool.New(functiontool.Config{
		Name:        "bash",
		Description: "Runs a shell command and returns its stdout, stderr, and exit code. Each invocation runs a single command. The command string is executed via sh -c, so pipes, redirects, and shell features work. The allow/deny command lists and path allowlist are defense-in-depth only and NOT a security sandbox; untrusted agents should run under process or container isolation.",
	}, func(ctx context.Context, args bashArgs) (map[string]any, error) {
		allCommands, parseErr := extractCommands(args.Command)
		if parseErr != nil {
			if len(cfg.AllowedCommands) > 0 {
				return nil, fmt.Errorf("bash: unable to parse command safely: %w", parseErr)
			}
			allCommands = naiveExtractCommands(args.Command)
		}

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

		timeout := args.Timeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}

		workDir := args.WorkDir
		if workDir == "" {
			workDir = cfg.WorkDir
		}

		if guard != nil && workDir != "" {
			if err := guard.Check(workDir); err != nil {
				return nil, fmt.Errorf("bash: %w", err)
			}
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
