package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "config", "user.email", "test@test.com")
	runCmd(t, dir, "git", "config", "user.name", "Test")
	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644)
	require.NoError(t, err)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "init")
	return dir
}

func defaultBranch(t *testing.T, dir string) string {
	t.Helper()
	c := exec.Command("git", "branch", "--show-current")
	c.Dir = dir
	out, err := c.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func setupBareRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runCmd(t, dir, "git", "init", "--bare")
	return dir
}

func runCmd(t *testing.T, dir string, cmd string, args ...string) {
	t.Helper()
	c := exec.Command(cmd, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	require.NoError(t, err, "command %s %v failed: %s", cmd, args, string(out))
}

func testConfig(dir string) Config {
	return Config{AllowedPaths: []string{dir}}
}

func TestGitStatus(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)
	tool, err := NewStatusTool(cfg)
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"workdir": dir,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
	assert.Contains(t, result["stdout"], "On branch")
}

func TestGitStatusShort(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)
	tool, err := NewStatusTool(cfg)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello\n"), 0o644)
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"short":  true,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
	assert.Contains(t, result["stdout"], "??")
}

func TestGitAddAndCommit(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)

	addTool, err := NewAddTool(cfg)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("new feature\n"), 0o644)
	require.NoError(t, err)

	result, err := addTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"paths":   []any{"feature.txt"},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])

	commitTool, err := NewCommitTool(cfg)
	require.NoError(t, err)

	result, err = commitTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"message": "add feature",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
	assert.Contains(t, result["stdout"], "add feature")
}

func TestGitAddAll(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)

	addTool, err := NewAddTool(cfg)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0o644)
	require.NoError(t, err)

	result, err := addTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"all":     true,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
}

func TestGitLog(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)

	tool, err := NewLogTool(cfg)
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"oneline": true,
		"count":   5,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
	assert.Contains(t, result["stdout"], "init")
}

func TestGitBranch(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)
	mainBranch := defaultBranch(t, dir)

	tool, err := NewBranchTool(cfg)
	require.NoError(t, err)

	result, err := tool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "current",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
	assert.Contains(t, result["stdout"], mainBranch)

	result, err = tool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "create",
		"name":    "feature",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])

	result, err = tool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "current",
	})
	require.NoError(t, err)
	assert.Contains(t, result["stdout"], "feature")

	result, err = tool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "list",
	})
	require.NoError(t, err)
	assert.Contains(t, result["stdout"], mainBranch)
	assert.Contains(t, result["stdout"], "feature")
}

func TestGitDiff(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)

	addTool, err := NewAddTool(cfg)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\nmodified\n"), 0o644)
	require.NoError(t, err)

	diffTool, err := NewDiffTool(cfg)
	require.NoError(t, err)

	result, err := diffTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"target":  "unstaged",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
	assert.Contains(t, result["stdout"], "modified")

	result, err = addTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"paths":   []any{"README.md"},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])

	result, err = diffTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"target":  "staged",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
	assert.Contains(t, result["stdout"], "modified")
}

func TestGitStash(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)

	err := os.WriteFile(filepath.Join(dir, "stash.txt"), []byte("stash me\n"), 0o644)
	require.NoError(t, err)

	addTool, err := NewAddTool(cfg)
	require.NoError(t, err)
	_, err = addTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"all":     true,
	})
	require.NoError(t, err)

	stashTool, err := NewStashTool(cfg)
	require.NoError(t, err)

	result, err := stashTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "save",
		"message": "wip",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])

	result, err = stashTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "list",
	})
	require.NoError(t, err)
	assert.Contains(t, result["stdout"], "wip")

	result, err = stashTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "pop",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
}

func TestGitCheckout(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)
	mainBranch := defaultBranch(t, dir)

	branchTool, err := NewBranchTool(cfg)
	require.NoError(t, err)
	_, err = branchTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "create",
		"name":    "other",
	})
	require.NoError(t, err)

	checkoutTool, err := NewCheckoutTool(cfg)
	require.NoError(t, err)

	result, err := checkoutTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "switch",
		"target":  mainBranch,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])

	result, err = branchTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "current",
	})
	require.NoError(t, err)
	assert.Contains(t, result["stdout"], mainBranch)
}

func TestGitCheckoutRestore(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)

	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# modified\n"), 0o644)
	require.NoError(t, err)

	checkoutTool, err := NewCheckoutTool(cfg)
	require.NoError(t, err)

	result, err := checkoutTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "restore",
		"target":  "HEAD",
		"paths":   []any{"README.md"},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])

	content, err := os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "# test")
}

func TestGitClone(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)
	cloneDir := filepath.Join(t.TempDir(), "cloned")
	cfg.AllowedPaths = append(cfg.AllowedPaths, cloneDir)

	cloneTool, err := NewCloneTool(cfg)
	require.NoError(t, err)

	result, err := cloneTool.Process(context.Background(), map[string]any{
		"url":       dir,
		"directory": cloneDir,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
	assert.Equal(t, cloneDir, result["directory"])

	_, err = os.Stat(filepath.Join(cloneDir, "README.md"))
	require.NoError(t, err)
}

func TestGitPushPull(t *testing.T) {
	bareDir := setupBareRepo(t)
	srcDir := setupGitRepo(t)
	mainBranch := defaultBranch(t, srcDir)

	runCmd(t, srcDir, "git", "remote", "add", "origin", bareDir)
	runCmd(t, srcDir, "git", "push", "-u", "origin", mainBranch)

	srcCfg := Config{AllowedPaths: []string{srcDir}}

	pushTool, err := NewPushTool(srcCfg)
	require.NoError(t, err)

	result, err := pushTool.Process(context.Background(), map[string]any{
		"workdir": srcDir,
		"remote":  "origin",
		"branch":  mainBranch,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])

	cloneDir := filepath.Join(t.TempDir(), "cloned")
	pullCfg := Config{AllowedPaths: []string{cloneDir}}
	runCmd(t, t.TempDir(), "git", "clone", bareDir, cloneDir)

	pullTool, err := NewPullTool(pullCfg)
	require.NoError(t, err)

	result, err = pullTool.Process(context.Background(), map[string]any{
		"workdir": cloneDir,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
}

func TestAllTools(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)

	ts, err := AllTools(cfg)
	require.NoError(t, err)

	tools := ts.Tools()
	assert.Len(t, tools, 11)

	names := make(map[string]bool)
	for _, tl := range tools {
		names[tl.Name()] = true
	}

	expected := []string{
		"git_clone", "git_pull", "git_push", "git_add",
		"git_branch", "git_commit", "git_stash",
		"git_status", "git_diff", "git_log", "git_checkout",
	}
	for _, name := range expected {
		assert.True(t, names[name], "missing tool %q", name)
	}
}

func TestPathguardBlocking(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := Config{AllowedPaths: []string{"/no/access"}}

	statusTool, err := NewStatusTool(cfg)
	require.NoError(t, err)

	_, err = statusTool.Process(context.Background(), map[string]any{
		"workdir": dir,
	})
	assert.Error(t, err)
}

func TestGitBranchDelete(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)
	mainBranch := defaultBranch(t, dir)

	branchTool, err := NewBranchTool(cfg)
	require.NoError(t, err)

	_, err = branchTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "create",
		"name":    "to-delete",
	})
	require.NoError(t, err)

	_, err = branchTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "switch",
		"name":    mainBranch,
	})
	require.NoError(t, err)

	result, err := branchTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"action":  "delete",
		"name":    "to-delete",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
}

func TestGitCommitAmend(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)

	commitTool, err := NewCommitTool(cfg)
	require.NoError(t, err)

	result, err := commitTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"message": "amended message",
		"amend":   true,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])

	logTool, err := NewLogTool(cfg)
	require.NoError(t, err)

	result, err = logTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"oneline": true,
		"count":   1,
	})
	require.NoError(t, err)
	assert.Contains(t, result["stdout"], "amended message")
}

func TestGitDiffStat(t *testing.T) {
	dir := setupGitRepo(t)
	cfg := testConfig(dir)

	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# modified\n"), 0o644)
	require.NoError(t, err)

	diffTool, err := NewDiffTool(cfg)
	require.NoError(t, err)

	result, err := diffTool.Process(context.Background(), map[string]any{
		"workdir": dir,
		"stat":    true,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result["exit_code"])
	assert.Contains(t, result["stdout"], "README.md")
}
