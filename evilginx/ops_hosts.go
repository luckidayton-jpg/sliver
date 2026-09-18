package evilginx

import (
	"context"

	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
)

// HostsList - list hosts the server has seen.
func (s *SliverBridge) HostsList() ([]HostSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.Hosts(context.Background(), &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	out := make([]HostSummary, 0, len(resp.Hosts))
	for _, h := range resp.Hosts {
		hs := HostSummary{
			ID:          h.ID,
			Hostname:    h.Hostname,
			HostUUID:    h.HostUUID,
			OSVersion:   h.OSVersion,
			Locale:      h.Locale,
			FirstContact: h.FirstContact,
		}
		for _, ioc := range h.IOCs {
			hs.IOCs = append(hs.IOCs, IOCSummary{
				ID:       ioc.ID,
				Path:     ioc.Path,
				FileHash: ioc.FileHash,
			})
		}
		out = append(out, hs)
	}
	return out, nil
}

// HostRm - remove a host record by id.
func (s *SliverBridge) HostRm(hostID string) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	_, err = c.HostRm(context.Background(), &clientpb.Host{ID: hostID})
	return err
}

type HostSummary struct {
	ID           string      `json:"id"`
	Hostname     string      `json:"hostname"`
	HostUUID     string      `json:"host_uuid"`
	OSVersion    string      `json:"os_version"`
	Locale       string      `json:"locale"`
	FirstContact int64       `json:"first_contact"`
	IOCs         []IOCSummary `json:"iocs"`
}

type IOCSummary struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	FileHash string `json:"file_hash"`
}