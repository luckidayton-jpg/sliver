package evilginx

import (
	"context"
	"fmt"
	"strings"

	"github.com/bishopfox/sliver/protobuf/clientpb"
)

// StartListener - start a new C2 listener. kind is one of:
// mtls | wg | dns | http | https | tcp-stager.
func (s *SliverBridge) StartListener(kind string, params ListenerParams) (*ListenerJobSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(kind) {
	case "mtls":
		resp, err := c.StartMTLSListener(context.Background(), &clientpb.MTLSListenerReq{
			Host: params.Host,
			Port: uint32(params.Port),
		})
		if err != nil {
			return nil, err
		}
		return &ListenerJobSummary{ID: int(resp.JobID), Type: "mtls", Port: params.Port}, nil

	case "wg":
		resp, err := c.StartWGListener(context.Background(), &clientpb.WGListenerReq{
			Port: uint32(params.Port),
		})
		if err != nil {
			return nil, err
		}
		return &ListenerJobSummary{ID: int(resp.JobID), Type: "wg", Port: params.Port}, nil

	case "dns":
		if len(params.Domains) == 0 {
			return nil, fmt.Errorf("dns listener requires at least one domain")
		}
		resp, err := c.StartDNSListener(context.Background(), &clientpb.DNSListenerReq{
			Domains:  params.Domains,
			Canaries: params.Canaries,
			Host:     params.Host,
			Port:     uint32(params.Port),
		})
		if err != nil {
			return nil, err
		}
		return &ListenerJobSummary{ID: int(resp.JobID), Type: "dns", Port: params.Port}, nil

	case "http", "https":
		secure := strings.ToLower(kind) == "https"
		req := &clientpb.HTTPListenerReq{
			Domain:         params.Domain,
			Host:           params.Host,
			Port:           uint32(params.Port),
			Secure:         secure,
			Website:        params.Website,
			ACME:           params.ACME,
			LongPollTimeout: params.LongPollTimeout,
			LongPollJitter: params.LongPollJitter,
		}
		var err error
		var job *clientpb.ListenerJob
		if secure {
			job, err = c.StartHTTPSListener(context.Background(), req)
		} else {
			job, err = c.StartHTTPListener(context.Background(), req)
		}
		if err != nil {
			return nil, err
		}
		return &ListenerJobSummary{ID: int(job.JobID), Type: strings.ToLower(kind), Port: params.Port}, nil

	case "tcp-stager", "tcp":
		resp, err := c.StartTCPStagerListener(context.Background(), &clientpb.StagerListenerReq{
			Protocol:    clientpb.StageProtocol_TCP,
			Host:        params.Host,
			Port:        uint32(params.Port),
			ProfileName: params.Profile,
		})
		if err != nil {
			return nil, err
		}
		return &ListenerJobSummary{ID: int(resp.JobID), Type: "tcp-stager", Port: params.Port}, nil

	default:
		return nil, fmt.Errorf("unknown listener type: %s", kind)
	}
}

// KillJob - stop a running listener by job id.
func (s *SliverBridge) KillJob(jobID uint32) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	resp, err := c.KillJob(context.Background(), &clientpb.KillJobReq{ID: jobID})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("server reported kill job %d failed", jobID)
	}
	return nil
}

// ListenerParams - parameters accepted by the listener starter.
type ListenerParams struct {
	Host            string   `json:"host"`
	Port            int      `json:"port"`
	Domain          string   `json:"domain"`
	Domains         []string `json:"domains"`
	Canaries        bool     `json:"canaries"`
	Website         string   `json:"website"`
	ACME            bool     `json:"acme"`
	LongPollTimeout int64    `json:"long_poll_timeout"`
	LongPollJitter  int64    `json:"long_poll_jitter"`
	Profile         string   `json:"profile"`
}

type ListenerJobSummary struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Port int    `json:"port"`
}