package evilginx

import (
	"context"
	"fmt"

	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
	"github.com/bishopfox/sliver/protobuf/rpcpb"
	"github.com/bishopfox/sliver/protobuf/sliverpb"
)

// GetSession - returns the full session record for a single session id.
func (s *SliverBridge) GetSession(id string) (*SessionDetail, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.GetSessions(context.Background(), &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	for _, sess := range resp.Sessions {
		if sess.ID == id {
			return toSessionDetail(sess), nil
		}
	}
	return nil, fmt.Errorf("session %s not found", id)
}

// KillSession - terminate a live session (optionally force).
func (s *SliverBridge) KillSession(id string, force bool) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	_, err = c.Kill(context.Background(), &sliverpb.KillReq{
		Request: &commonpb.Request{SessionID: id, Timeout: defaultRPCTimeout},
		Force:   force,
	})
	return err
}

// RenameSession - rename a session (or beacon).
func (s *SliverBridge) RenameSession(sessionID, beaconID, name string) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	_, err = c.Rename(context.Background(), &clientpb.RenameReq{
		SessionID: sessionID,
		BeaconID:  beaconID,
		Name:      name,
	})
	return err
}

// ListBeacons - summarize registered beacons.
func (s *SliverBridge) ListBeacons() ([]BeaconSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.GetBeacons(context.Background(), &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	out := make([]BeaconSummary, 0, len(resp.Beacons))
	for _, b := range resp.Beacons {
		out = append(out, BeaconSummary{
			ID:               b.ID,
			Name:             b.Name,
			Hostname:         b.Hostname,
			Username:         b.Username,
			OS:               b.OS,
			Arch:             b.Arch,
			Transport:        b.Transport,
			RemoteAddress:    b.RemoteAddress,
			PID:              b.PID,
			Filename:         b.Filename,
			LastCheckin:      b.LastCheckin,
			NextCheckin:      b.NextCheckin,
			ActiveC2:         b.ActiveC2,
			Version:          b.Version,
			IsDead:           b.IsDead,
			Interval:         b.Interval,
			Jitter:           b.Jitter,
			ReconnectInterval: b.ReconnectInterval,
			Locale:           b.Locale,
			FirstContact:     b.FirstContact,
			TasksCount:       b.TasksCount,
			TasksCompleted:   b.TasksCountCompleted,
		})
	}
	return out, nil
}

// BeaconTasks - list the task ledger for a beacon.
func (s *SliverBridge) BeaconTasks(beaconID string) ([]BeaconTaskSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.GetBeaconTasks(context.Background(), &clientpb.Beacon{ID: beaconID})
	if err != nil {
		return nil, err
	}
	out := make([]BeaconTaskSummary, 0, len(resp.Tasks))
	for _, t := range resp.Tasks {
		out = append(out, BeaconTaskSummary{
			ID:          t.ID,
			BeaconID:    t.BeaconID,
			State:       t.State,
			CreatedAt:   t.CreatedAt,
			SentAt:      t.SentAt,
			CompletedAt: t.CompletedAt,
			Description: t.Description,
			Status:      string(t.Response),
		})
	}
	return out, nil
}

// KillBeacon - request the beacon's implant terminate (queued as a task).
func (s *SliverBridge) KillBeacon(beaconID string) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	_, err = c.Kill(context.Background(), &sliverpb.KillReq{
		Request: &commonpb.Request{BeaconID: beaconID, Timeout: defaultRPCTimeout},
		Force:   true,
	})
	return err
}

// RmBeacon - remove a beacon from the server database without killing it.
func (s *SliverBridge) RmBeacon(beaconID string) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	_, err = c.RmBeacon(context.Background(), &clientpb.Beacon{ID: beaconID})
	return err
}

// GetOperators - list the operator accounts registered on the server.
func (s *SliverBridge) GetOperators() ([]OperatorSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.GetOperators(context.Background(), &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	out := make([]OperatorSummary, 0, len(resp.Operators))
	for _, op := range resp.Operators {
		out = append(out, OperatorSummary{Name: op.Name, Online: op.Online})
	}
	return out, nil
}

// --- typed summaries ---

type SessionDetail struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Hostname         string   `json:"hostname"`
	UUID             string   `json:"uuid"`
	Username         string   `json:"username"`
	UID              string   `json:"uid"`
	GID              string   `json:"gid"`
	OS               string   `json:"os"`
	Arch             string   `json:"arch"`
	Transport        string   `json:"transport"`
	RemoteAddress    string   `json:"remote_address"`
	PID              int32    `json:"pid"`
	Filename         string   `json:"filename"`
	LastCheckin      int64    `json:"last_checkin"`
	ActiveC2         string   `json:"active_c2"`
	Version          string   `json:"version"`
	Evasion          bool     `json:"evasion"`
	IsDead           bool     `json:"is_dead"`
	ReconnectInterval int64   `json:"reconnect_interval"`
	ProxyURL         string   `json:"proxy_url"`
	Burned           bool     `json:"burned"`
	Extensions       []string `json:"extensions"`
	PeerID           int64    `json:"peer_id"`
	Locale           string   `json:"locale"`
	FirstContact     int64    `json:"first_contact"`
	Integrity        string   `json:"integrity"`
}

func toSessionDetail(sess *clientpb.Session) *SessionDetail {
	return &SessionDetail{
		ID:                sess.ID,
		Name:              sess.Name,
		Hostname:          sess.Hostname,
		UUID:              sess.UUID,
		Username:          sess.Username,
		UID:               sess.UID,
		GID:               sess.GID,
		OS:                sess.OS,
		Arch:              sess.Arch,
		Transport:         sess.Transport,
		RemoteAddress:     sess.RemoteAddress,
		PID:               sess.PID,
		Filename:          sess.Filename,
		LastCheckin:       sess.LastCheckin,
		ActiveC2:          sess.ActiveC2,
		Version:           sess.Version,
		Evasion:           sess.Evasion,
		IsDead:            sess.IsDead,
		ReconnectInterval: sess.ReconnectInterval,
		ProxyURL:          sess.ProxyURL,
		Burned:            sess.Burned,
		Extensions:        sess.Extensions,
		PeerID:            sess.PeerID,
		Locale:            sess.Locale,
		FirstContact:      sess.FirstContact,
		Integrity:         sess.Integrity,
	}
}

type BeaconSummary struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Hostname          string `json:"hostname"`
	Username          string `json:"username"`
	OS                string `json:"os"`
	Arch              string `json:"arch"`
	Transport         string `json:"transport"`
	RemoteAddress     string `json:"remote_address"`
	PID               int32  `json:"pid"`
	Filename          string `json:"filename"`
	LastCheckin       int64  `json:"last_checkin"`
	NextCheckin       int64  `json:"next_checkin"`
	ActiveC2          string `json:"active_c2"`
	Version           string `json:"version"`
	IsDead            bool   `json:"is_dead"`
	Interval          int64  `json:"interval"`
	Jitter            int64  `json:"jitter"`
	ReconnectInterval int64  `json:"reconnect_interval"`
	Locale            string `json:"locale"`
	FirstContact      int64  `json:"first_contact"`
	TasksCount        int64  `json:"tasks_count"`
	TasksCompleted    int64  `json:"tasks_completed"`
}

type BeaconTaskSummary struct {
	ID          string `json:"id"`
	BeaconID    string `json:"beacon_id"`
	State       string `json:"state"`
	CreatedAt   int64  `json:"created_at"`
	SentAt      int64  `json:"sent_at"`
	CompletedAt int64  `json:"completed_at"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type OperatorSummary struct {
	Name   string `json:"name"`
	Online bool   `json:"online"`
}

// --- helpers ---

func (s *SliverBridge) requireClient() (rpcpb.SliverRPCClient, error) {
	if !s.Running() || s.client == nil {
		return nil, fmt.Errorf("sliver not running")
	}
	return s.client, nil
}