package evilginx

import (
	"context"
	"fmt"

	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
)

// LootList - summarize the loot database.
func (s *SliverBridge) LootList() ([]LootSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.LootAll(context.Background(), &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	out := make([]LootSummary, 0, len(resp.Loot))
	for _, l := range resp.Loot {
		out = append(out, LootSummary{
			ID:       l.ID,
			Name:     l.Name,
			FileType: fileTypeString(l.FileType),
			Size:     l.Size,
		})
	}
	return out, nil
}

// LootContent - fetch the stored file contents for a loot entry.
func (s *SliverBridge) LootContent(lootID string) (*LootContentResult, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.LootContent(context.Background(), &clientpb.Loot{ID: lootID})
	if err != nil {
		return nil, err
	}
	if resp.File == nil {
		return nil, fmt.Errorf("loot entry has no file content")
	}
	return &LootContentResult{
		ID:         resp.ID,
		Name:       resp.Name,
		FileName:   resp.File.Name,
		FileType:   fileTypeString(resp.FileType),
		Data:       resp.File.Data,
	}, nil
}

// LootAddFile - add a file artifact to the loot database.
func (s *SliverBridge) LootAddFile(lootName, fileName string, fileType string, data []byte) (*LootSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	if lootName == "" {
		lootName = fileName
	}
	ft, err := parseFileType(fileType)
	if err != nil {
		return nil, err
	}
	resp, err := c.LootAdd(context.Background(), &clientpb.Loot{
		Name:     lootName,
		FileType: ft,
		Size:     int64(len(data)),
		File:     &commonpb.File{Name: fileName, Data: data},
	})
	if err != nil {
		return nil, err
	}
	return &LootSummary{
		ID:       resp.ID,
		Name:     resp.Name,
		FileType: fileTypeString(resp.FileType),
		Size:     resp.Size,
	}, nil
}

// LootRm - remove a loot entry by id.
func (s *SliverBridge) LootRm(lootID string) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	_, err = c.LootRm(context.Background(), &clientpb.Loot{ID: lootID})
	return err
}

type LootSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FileType string `json:"file_type"`
	Size     int64  `json:"size"`
}

type LootContentResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FileName string `json:"file_name"`
	FileType string `json:"file_type"`
	Data     []byte `json:"data"`
}

func fileTypeString(ft clientpb.FileType) string {
	switch ft {
	case clientpb.FileType_BINARY:
		return "binary"
	case clientpb.FileType_TEXT:
		return "text"
	default:
		return "unknown"
	}
}

func parseFileType(s string) (clientpb.FileType, error) {
	switch s {
	case "binary":
		return clientpb.FileType_BINARY, nil
	case "text":
		return clientpb.FileType_TEXT, nil
	default:
		return clientpb.FileType_NO_FILE, fmt.Errorf("unknown file type: %s", s)
	}
}