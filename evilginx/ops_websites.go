package evilginx

import (
	"context"
	"fmt"

	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
)

// WebsitesList - list website hosting configurations.
func (s *SliverBridge) WebsitesList() ([]WebsiteSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.Websites(context.Background(), &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	out := make([]WebsiteSummary, 0, len(resp.Websites))
	for _, w := range resp.Websites {
		ws := WebsiteSummary{ID: w.ID, Name: w.Name}
		for path, content := range w.Contents {
			ws.Contents = append(ws.Contents, ContentSummary{
				Path:        path,
				ContentType: content.ContentType,
				Size:        int64(content.Size),
				Sha256:      content.Sha256,
			})
		}
		out = append(out, ws)
	}
	return out, nil
}

// WebsiteAdd - create a new (empty) website by name.
func (s *SliverBridge) WebsiteAdd(name string) (*WebsiteSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("website name is required")
	}
	resp, err := c.WebsiteAddContent(context.Background(), &clientpb.WebsiteAddContent{
		Name:     name,
		Contents: map[string]*clientpb.WebContent{},
	})
	if err != nil {
		return nil, err
	}
	return &WebsiteSummary{ID: resp.ID, Name: resp.Name}, nil
}

// WebsiteRemove - delete a website.
func (s *SliverBridge) WebsiteRemove(name string) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	_, err = c.WebsiteRemove(context.Background(), &clientpb.Website{Name: name})
	return err
}

// WebsiteAddContent - add or update a content entry on a website.
func (s *SliverBridge) WebsiteAddContent(name, path, contentType string, data []byte) (*WebsiteSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("content path is required")
	}
	resp, err := c.WebsiteAddContent(context.Background(), &clientpb.WebsiteAddContent{
		Name: name,
		Contents: map[string]*clientpb.WebContent{
			path: {
				Path:        path,
				ContentType: contentType,
				Content:     data,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return &WebsiteSummary{ID: resp.ID, Name: resp.Name}, nil
}

// WebsiteRemoveContent - delete one or more content paths from a website.
func (s *SliverBridge) WebsiteRemoveContent(name string, paths []string) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	_, err = c.WebsiteRemoveContent(context.Background(), &clientpb.WebsiteRemoveContent{
		Name:  name,
		Paths: paths,
	})
	return err
}

type WebsiteSummary struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Contents []ContentSummary `json:"contents"`
}

type ContentSummary struct {
	Path        string `json:"path"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Sha256      string `json:"sha256"`
}