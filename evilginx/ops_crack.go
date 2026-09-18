package evilginx

import (
	"context"
	"fmt"
	"strings"

	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
)

// CrackJobsList - list password-cracking jobs.
func (s *SliverBridge) CrackJobsList() ([]CrackJobSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.CrackJobs(context.Background(), &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	out := make([]CrackJobSummary, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		out = append(out, *toCrackJobSummary(j))
	}
	return out, nil
}

// CrackStart - dispatch a hashcat job against the crackstation fleet.
func (s *SliverBridge) CrackStart(hashMode uint32, attackMode int, hashes, positional []string) (*CrackJobSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	if len(hashes) == 0 {
		return nil, fmt.Errorf("at least one hash is required")
	}
	cmd := &clientpb.CrackCommand{
		Hashes:              hashes,
		PositionalArguments: positional,
	}
	if hashMode != 0 {
		mode := hashMode
		cmd.HashMode = &mode
	} else {
		cmd.HashType = clientpb.HashType_INVALID
	}
	switch attackMode {
	case 0:
		cmd.AttackMode = clientpb.CrackAttackMode_STRAIGHT
	case 1:
		cmd.AttackMode = clientpb.CrackAttackMode_COMBINATION
	case 3:
		cmd.AttackMode = clientpb.CrackAttackMode_BRUTEFORCE
	case 6:
		cmd.AttackMode = clientpb.CrackAttackMode_HYBRID_WORDLIST_MASK
	case 7:
		cmd.AttackMode = clientpb.CrackAttackMode_HYBRID_MASK_WORDLIST
	default:
		cmd.AttackMode = clientpb.CrackAttackMode_STRAIGHT
	}

	resp, err := c.Crack(context.Background(), cmd)
	if err != nil {
		return nil, err
	}
	if resp.Job == nil {
		return nil, fmt.Errorf("crack dispatch returned no job")
	}
	return toCrackJobSummary(resp.Job), nil
}

// CrackJobCancel - cancel a password-cracking job.
func (s *SliverBridge) CrackJobCancel(jobID string) (*CrackJobSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.CrackJobCancel(context.Background(), &clientpb.CrackJob{ID: jobID})
	if err != nil {
		return nil, err
	}
	return toCrackJobSummary(resp), nil
}

// CrackJobDelete - delete a completed/failed job record.
func (s *SliverBridge) CrackJobDelete(jobID string) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	_, err = c.CrackJobDelete(context.Background(), &clientpb.CrackJob{ID: jobID})
	return err
}

// CrackJobSummary - digested view of a crack job for the dashboard.
type CrackJobSummary struct {
	ID          string             `json:"id"`
	CreatedAt   string             `json:"created_at"`
	CompletedAt string             `json:"completed_at"`
	Status      string             `json:"status"`
	Err         string             `json:"err"`
	ResultCount uint64             `json:"result_count"`
	Command     CrackCommandBrief  `json:"command"`
}

// CrackCommandBrief - readable command snapshot.
type CrackCommandBrief struct {
	AttackMode      string   `json:"attack_mode"`
	HashMode        uint32   `json:"hash_mode,omitempty"`
	Hashes          []string `json:"hashes"`
	PositionalArgs  []string `json:"positional_arguments"`
}

func toCrackJobSummary(j *clientpb.CrackJob) *CrackJobSummary {
	out := &CrackJobSummary{
		ID:          j.ID,
		CreatedAt:   j.CreatedAt,
		CompletedAt: j.CompletedAt,
		Status:      crackJobStatusString(j.Status),
		Err:         j.Err,
		ResultCount: j.ResultCount,
	}
	if j.Command != nil {
		brief := CrackCommandBrief{
			AttackMode:     j.Command.AttackMode.String(),
			Hashes:         j.Command.Hashes,
			PositionalArgs: j.Command.PositionalArguments,
		}
		if j.Command.HashMode != nil {
			brief.HashMode = *j.Command.HashMode
		}
		out.Command = brief
	}
	return out
}

func crackJobStatusString(st clientpb.CrackJobStatus) string {
	return strings.ToLower(st.String())
}