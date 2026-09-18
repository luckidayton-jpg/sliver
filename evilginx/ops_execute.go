package evilginx

import (
	"context"
	"fmt"

	"github.com/bishopfox/sliver/protobuf/commonpb"
	"github.com/bishopfox/sliver/protobuf/sliverpb"
)

// ExecResult - result of a remote process execution on a session.
type ExecResult struct {
	Pid    uint32 `json:"pid"`
	Status uint32 `json:"status"`
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

// SessionExecute runs a program (shell, powershell, script...) on a session.
// When background is true the RPC returns as soon as the process is spawned,
// which is the right mode for asynchronous payloads like agent installers; and
// in that case stdout/stderr are not captured (Sliver streams them to the
// server instead). Foreground mode blocks with a caller-supplied timeout.
func (s *SliverBridge) SessionExecute(sessionID, path string, args []string, background bool, timeout int64) (*ExecResult, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	if timeout <= 0 || timeout > 600 {
		timeout = 300
	}
	resp, err := c.Execute(context.Background(), &sliverpb.ExecuteReq{
		Path:       path,
		Args:       args,
		Output:     !background,
		Background: background,
		Request:    &commonpb.Request{SessionID: sessionID, Timeout: timeout},
	})
	if err != nil {
		return nil, fmt.Errorf("session execute: %w", err)
	}
	return &ExecResult{
		Pid:    resp.Pid,
		Status: resp.Status,
		Stdout: string(resp.Stdout),
		Stderr: string(resp.Stderr),
	}, nil
}
