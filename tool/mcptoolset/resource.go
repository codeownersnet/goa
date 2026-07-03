package mcptoolset

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/codeownersnet/goa/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (ts *Toolset) Resources() []tool.ResourceInfo {
	return ts.resources
}

func (ts *Toolset) ReadResource(ctx context.Context, uri string) ([]byte, error) {
	result, err := ts.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
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
	var resources []tool.ResourceInfo
	for res, err := range ts.session.Resources(ctx, nil) {
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
	ts.resources = resources
	return nil
}

var _ tool.ResourceProvider = (*Toolset)(nil)
