package artifact

import (
	"context"
	"fmt"
	"strings"
)

type Saver interface {
	Save(ctx context.Context, req *SaveRequest) (*SaveResponse, error)
}

type Loader interface {
	Load(ctx context.Context, req *LoadRequest) (*LoadResponse, error)
}

type Deleter interface {
	Delete(ctx context.Context, req *DeleteRequest) error
}

type Lister interface {
	List(ctx context.Context, req *ListRequest) (*ListResponse, error)
}

type Versioner interface {
	Versions(ctx context.Context, req *VersionsRequest) (*VersionsResponse, error)
}

type ArtifactVersionGetter interface {
	GetArtifactVersion(ctx context.Context, req *GetArtifactVersionRequest) (*GetArtifactVersionResponse, error)
}

type Service interface {
	Saver
	Loader
	Deleter
	Lister
	Versioner
	ArtifactVersionGetter
}

type SaveRequest struct {
	AppName, UserID, SessionID, FileName string
	Data                                 []byte
	MIMEType                             string
	Version                              int64
}

type SaveResponse struct {
	Version int64
}

type LoadRequest struct {
	AppName, UserID, SessionID, FileName string
	Version                              int64
}

type LoadResponse struct {
	Data     []byte
	MIMEType string
}

type DeleteRequest struct {
	AppName, UserID, SessionID, FileName string
	Version                              int64
}

type ListRequest struct {
	AppName, UserID, SessionID string
}

type ListResponse struct {
	FileNames []string
}

type VersionsRequest struct {
	AppName, UserID, SessionID, FileName string
}

type VersionsResponse struct {
	Versions []int64
}

type ArtifactVersion struct {
	Version    int64
	MIMEType   string
	CreateTime float64
}

type GetArtifactVersionRequest struct {
	AppName, UserID, SessionID, FileName string
	Version                              int64
}

type GetArtifactVersionResponse struct {
	ArtifactVersion *ArtifactVersion
}

func validateFileName(name string) error {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("filename cannot contain path separators")
	}
	return nil
}

func (req *SaveRequest) Validate() error {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" || req.FileName == "" {
		return fmt.Errorf("missing required fields")
	}
	return validateFileName(req.FileName)
}

func (req *LoadRequest) Validate() error {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" || req.FileName == "" {
		return fmt.Errorf("missing required fields")
	}
	return validateFileName(req.FileName)
}

func (req *DeleteRequest) Validate() error {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" || req.FileName == "" {
		return fmt.Errorf("missing required fields")
	}
	return validateFileName(req.FileName)
}

func (req *ListRequest) Validate() error {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" {
		return fmt.Errorf("missing required fields")
	}
	return nil
}

func (req *VersionsRequest) Validate() error {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" || req.FileName == "" {
		return fmt.Errorf("missing required fields")
	}
	return validateFileName(req.FileName)
}

func (req *GetArtifactVersionRequest) Validate() error {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" || req.FileName == "" {
		return fmt.Errorf("missing required fields")
	}
	return validateFileName(req.FileName)
}
