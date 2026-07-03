package workflow

import (
	"testing"

	"github.com/codeownersnet/goa/tool/mcptoolset"
	"github.com/stretchr/testify/assert"
)

func TestIsMCPWildcard(t *testing.T) {
	assert.True(t, isMCPWildcard("mcp:filesystem"))
	assert.True(t, isMCPWildcard("mcp:a"))
	assert.False(t, isMCPWildcard("mcp:"))
	assert.False(t, isMCPWildcard("bash"))
	assert.False(t, isMCPWildcard("mcp"))
	assert.False(t, isMCPWildcard(""))
}

func TestMCPServerName(t *testing.T) {
	assert.Equal(t, "filesystem", mcpServerName("mcp:filesystem"))
	assert.Equal(t, "my-server", mcpServerName("mcp:my-server"))
}

func TestMCPServerYAMLOptionsCommand(t *testing.T) {
	opts, err := mcpServerYAMLOptions("fs", mcpServerYAML{
		Command:        "npx -y @mcp/server-filesystem /tmp",
		ConnectTimeout: "10s",
		ToolTimeout:    "120s",
		Env:            []string{"FOO=bar"},
	})
	assert.NoError(t, err)
	assert.Len(t, opts, 5)
}

func TestMCPServerYAMLOptionsURL(t *testing.T) {
	opts, err := mcpServerYAMLOptions("api", mcpServerYAML{
		URL:     "https://mcp.example.com/sse",
		Headers: map[string]string{"Authorization": "Bearer x"},
	})
	assert.NoError(t, err)
	assert.Len(t, opts, 3)
}

func TestMCPServerYAMLOptionsBadDuration(t *testing.T) {
	_, err := mcpServerYAMLOptions("srv", mcpServerYAML{
		URL:            "https://example.com",
		ConnectTimeout: "not-a-duration",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connect_timeout")
}

func TestResolveMCPServersEmpty(t *testing.T) {
	resolver, err := resolveMCPServers(t.Context(), nil)
	assert.NoError(t, err)
	assert.Empty(t, resolver.toolsets)

	resolver, err = resolveMCPServers(t.Context(), map[string]mcpServerYAML{})
	assert.NoError(t, err)
	assert.Empty(t, resolver.toolsets)
}

func TestMCPRResolverLookupEmpty(t *testing.T) {
	resolver := &mcpResolver{toolsets: make(map[string]*mcptoolset.Toolset)}
	_, ok := resolver.lookupTool("anything")
	assert.False(t, ok)
}

func TestMCPResolverCloseAllEmpty(t *testing.T) {
	resolver := &mcpResolver{toolsets: make(map[string]*mcptoolset.Toolset)}
	err := resolver.closeAll()
	assert.NoError(t, err)
}

func TestMCPResolverToolsetNotFound(t *testing.T) {
	resolver := &mcpResolver{toolsets: make(map[string]*mcptoolset.Toolset)}
	_, ok := resolver.toolset("nonexistent")
	assert.False(t, ok)
}

func TestMCPResolverAllToolsetsEmpty(t *testing.T) {
	resolver := &mcpResolver{toolsets: make(map[string]*mcptoolset.Toolset)}
	all := resolver.allToolsets()
	assert.Empty(t, all)
}
