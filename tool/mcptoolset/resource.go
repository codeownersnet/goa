package mcptoolset

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (ts *Toolset) Resources() []tool.ResourceInfo {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return append([]tool.ResourceInfo(nil), ts.resources...)
}

func (ts *Toolset) ResourceTemplates() []tool.ResourceTemplateInfo {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return append([]tool.ResourceTemplateInfo(nil), ts.resourceTemplates...)
}

func (ts *Toolset) ReadResource(ctx context.Context, uri string) ([]byte, error) {
	session := ts.currentSession()
	if session == nil {
		return nil, errNotConnected
	}

	result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		return nil, fmt.Errorf("mcptoolset %q: read resource %q: %w", ts.name, uri, err)
	}

	if len(result.Contents) == 0 {
		return nil, fmt.Errorf("mcptoolset %q: resource %q returned no contents", ts.name, uri)
	}

	var data []byte
	for _, rc := range result.Contents {
		if rc.Text != "" {
			data = append(data, []byte(rc.Text)...)
		} else if len(rc.Blob) > 0 {
			decoded, err := base64.StdEncoding.AppendDecode(nil, rc.Blob)
			if err != nil {
				data = append(data, rc.Blob...)
			} else {
				data = append(data, decoded...)
			}
		}
	}

	return data, nil
}

func (ts *Toolset) discoverResources(ctx context.Context) error {
	session := ts.currentSession()
	if session == nil {
		return errNotConnected
	}

	var resources []tool.ResourceInfo
	for res, err := range session.Resources(ctx, nil) {
		if err != nil {
			return fmt.Errorf("list resources: %w", err)
		}
		resources = append(resources, tool.ResourceInfo{
			Name:        res.Name,
			URI:         res.URI,
			Description: res.Description,
			MimeType:    res.MIMEType,
		})
	}

	ts.mu.Lock()
	ts.resources = resources
	ts.mu.Unlock()
	return nil
}

func (ts *Toolset) discoverResourceTemplates(ctx context.Context) error {
	session := ts.currentSession()
	if session == nil {
		return errNotConnected
	}

	var templates []tool.ResourceTemplateInfo
	for tpl, err := range session.ResourceTemplates(ctx, nil) {
		if err != nil {
			return fmt.Errorf("list resource templates: %w", err)
		}
		templates = append(templates, tool.ResourceTemplateInfo{
			Name:        tpl.Name,
			URITemplate: tpl.URITemplate,
			Description: tpl.Description,
			MimeType:    tpl.MIMEType,
		})
	}

	ts.mu.Lock()
	ts.resourceTemplates = templates
	ts.mu.Unlock()
	return nil
}

var _ tool.ResourceProvider = (*Toolset)(nil)
var _ tool.ResourceTemplateProvider = (*Toolset)(nil)
