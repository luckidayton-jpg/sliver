package rpc

/*
	Sliver Implant Framework
	Copyright (C) 2019  Bishop Fox

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"

	"github.com/bishopfox/sliver/client/constants"
	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
	"github.com/bishopfox/sliver/server/c2"
	"github.com/bishopfox/sliver/server/core"
	"github.com/bishopfox/sliver/server/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultMTLSPort    = 4444
	defaultWGPort      = 53
	defaultWGNPort     = 8888
	defaultWGKeyExPort = 1337
	defaultDNSPort     = 53
	defaultHTTPPort    = 80
	defaultHTTPSPort   = 443
)

var (
	// ErrInvalidPort - Invalid TCP port number
	ErrInvalidPort = status.Error(codes.InvalidArgument, "invalid listener port")
)

// GetJobs - List jobs
func (rpc *Server) GetJobs(ctx context.Context, _ *commonpb.Empty) (*clientpb.Jobs, error) {
	jobs := &clientpb.Jobs{
		Active: []*clientpb.Job{},
	}
	for _, job := range core.Jobs.All() {
		jobs.Active = append(jobs.Active, &clientpb.Job{
			ID:          uint32(job.ID),
			Name:        job.Name,
			Description: job.Description,
			Protocol:    job.Protocol,
			Port:        uint32(job.Port),
			Domains:     job.Domains,
			ProfileName: job.ProfileName,
		})
	}
	return jobs, nil
}

// Restart Jobs - Reload jobs
func (rpc *Server) RestartJobs(ctx context.Context, restartJobReq *clientpb.RestartJobReq) (*commonpb.Empty, error) {
	// reload jobs to include new profile
	for _, jobID := range restartJobReq.JobIDs {
		job := core.Jobs.Get(int(jobID))
		listenerJob, err := db.ListenerByJobID(jobID)
		if err != nil {
			return &commonpb.Empty{}, rpcError(err)
		}
		job.JobCtrl <- true
		job, err = c2.StartHTTPListenerJob(listenerJob.HTTPConf)
		if err != nil {
			return &commonpb.Empty{}, rpcError(err)
		}
		if err := db.UpdateListenerJobID(jobID, uint32(job.ID)); err != nil {
			return &commonpb.Empty{}, rpcError(err)
		}
	}
	return &commonpb.Empty{}, nil
}

// KillJob - Kill a server-side job
func (rpc *Server) KillJob(ctx context.Context, kill *clientpb.KillJobReq) (*clientpb.KillJob, error) {
	job := core.Jobs.Get(int(kill.ID))
	killJob := &clientpb.KillJob{}
	var err error = nil
	if job != nil {
		job.JobCtrl <- true
		killJob.ID = uint32(job.ID)
		killJob.Success = true
		err = db.DeleteListener(killJob.ID)
		if err != nil {
			return nil, rpcError(err)
		}
	} else {
		killJob.Success = false
		err = status.Error(codes.NotFound, "invalid job ID")
	}
	return killJob, err
}

// StartMTLSListener - Start an MTLS listener
func (rpc *Server) StartMTLSListener(ctx context.Context, req *clientpb.MTLSListenerReq) (*clientpb.ListenerJob, error) {
	if 65535 <= req.Port {
		return nil, ErrInvalidPort
	}
	if req.Port == 0 {
		req.Port = defaultMTLSPort
	}

	err := PortInUse(req.Port)
	if err != nil {
		return nil, rpcError(err)
	}

	job, err := c2.StartMTLSListenerJob(req)
	if err != nil {
		return nil, rpcError(err)
	}

	listenerJob := &clientpb.ListenerJob{
		JobID:    uint32(job.ID),
		Type:     constants.MtlsStr,
		MTLSConf: req,
	}
	err = db.SaveC2Listener(listenerJob)
	if err != nil {
		return nil, rpcError(err)
	}

	return &clientpb.ListenerJob{JobID: uint32(job.ID)}, nil
}

// StartWGListener - Start a Wireguard listener
func (rpc *Server) StartWGListener(ctx context.Context, req *clientpb.WGListenerReq) (*clientpb.ListenerJob, error) {

	if 65535 <= req.Port || 65535 <= req.NPort || 65535 <= req.KeyPort {
		return nil, ErrInvalidPort
	}
	if req.Port == 0 {
		req.Port = defaultWGPort
	}

	if req.NPort == 0 {
		req.NPort = defaultWGNPort
	}

	if req.KeyPort == 0 {
		req.KeyPort = defaultWGKeyExPort
	}

	if req.NPort == req.KeyPort {
		return nil, status.Error(codes.InvalidArgument, "wg nport and key-port must be different")
	}

	err := PortInUse(req.Port)
	if err != nil {
		return nil, rpcError(err)
	}

	job, err := c2.StartWGListenerJob(req)
	if err != nil {
		return nil, rpcError(err)
	}

	listenerJob := &clientpb.ListenerJob{
		JobID:  uint32(job.ID),
		Type:   constants.WGStr,
		WGConf: req,
	}
	err = db.SaveC2Listener(listenerJob)
	if err != nil {
		return nil, rpcError(err)
	}

	return &clientpb.ListenerJob{JobID: uint32(job.ID)}, nil
}

// StartDNSListener - Start a DNS listener TODO: respect request's Host specification
func (rpc *Server) StartDNSListener(ctx context.Context, req *clientpb.DNSListenerReq) (*clientpb.ListenerJob, error) {
	err := PortInUse(req.Port)
	if err != nil {
		return nil, rpcError(err)
	}

	job, err := c2.StartDNSListenerJob(req)
	if err != nil {
		return nil, rpcError(err)
	}

	listenerJob := &clientpb.ListenerJob{
		JobID:   uint32(job.ID),
		Type:    constants.DnsStr,
		DNSConf: req,
	}
	err = db.SaveC2Listener(listenerJob)
	if err != nil {
		return nil, rpcError(err)
	}

	return &clientpb.ListenerJob{JobID: uint32(job.ID)}, nil
}

// StartHTTPSListener - Start an HTTPS listener
func (rpc *Server) StartHTTPSListener(ctx context.Context, req *clientpb.HTTPListenerReq) (*clientpb.ListenerJob, error) {
	if 65535 <= req.Port {
		return nil, ErrInvalidPort
	}
	if req.Port == 0 {
		req.Port = defaultHTTPSPort
	}

	err := PortInUse(req.Port)
	if err != nil {
		return nil, rpcError(err)
	}

	job, err := c2.StartHTTPListenerJob(req)
	if err != nil {
		return nil, rpcError(err)
	}

	listenerJob := &clientpb.ListenerJob{
		JobID:    uint32(job.ID),
		Type:     constants.HttpsStr,
		HTTPConf: req,
	}
	err = db.SaveC2Listener(listenerJob)
	if err != nil {
		return nil, rpcError(err)
	}

	return &clientpb.ListenerJob{JobID: uint32(job.ID)}, nil
}

// StartHTTPListener - Start an HTTP listener
func (rpc *Server) StartHTTPListener(ctx context.Context, req *clientpb.HTTPListenerReq) (*clientpb.ListenerJob, error) {
	if 65535 <= req.Port {
		return nil, ErrInvalidPort
	}
	if req.Port == 0 {
		req.Port = defaultHTTPPort
	}

	err := PortInUse(req.Port)
	if err != nil {
		return nil, rpcError(err)
	}

	job, err := c2.StartHTTPListenerJob(req)
	if err != nil {
		return nil, rpcError(err)
	}

	listenerJob := &clientpb.ListenerJob{
		JobID:    uint32(job.ID),
		Type:     constants.HttpStr,
		HTTPConf: req,
	}
	err = db.SaveC2Listener(listenerJob)
	if err != nil {
		return nil, rpcError(err)
	}

	return &clientpb.ListenerJob{JobID: uint32(job.ID)}, nil
}

// PortInUse reports whether a listener port cannot be taken.
//
// It asks the operating system rather than the database. It used to walk the stored
// listener records, which made a stale record permanently block a port: a record
// outlives the socket that backed it across a restart, so after one restart the port
// was "in use" with nothing listening on it, the console listed no listeners, and
// creating one on that port was refused. The operator is left unable to start the C2
// at all, with an error naming a port that is free.
//
// A socket is the thing that occupies a port. If the bind succeeds, the port is
// available, and whatever the database believes about a job whose socket is gone is
// history rather than an occupancy. A live listener still fails the bind and is still
// correctly refused, so nothing is lost by asking the OS.
//
// There is a narrow window between releasing this probe socket and the real listener
// binding, but the previous check had the same window and more: it was a different
// question from the one being asked.
func PortInUse(newPort uint32) error {
	if newPort == 0 {
		return status.Error(codes.InvalidArgument, "a port is required")
	}
	// Both a wildcard and a loopback address are probed, because they are not
	// equivalent: on Linux and macOS, binding :P succeeds while something already holds
	// 127.0.0.1:P. A wildcard-only probe therefore reports a loopback-bound listener as
	// free, and two listeners end up fighting for the port -- which is the exact
	// outcome this function exists to prevent.
	//
	// Order matters only for which address names the error, so wildcard first: that is
	// the address a listener is normally bound to.
	for _, addr := range []string{fmt.Sprintf(":%d", newPort), fmt.Sprintf("127.0.0.1:%d", newPort)} {
		probe, err := net.Listen("tcp", addr)
		if err != nil {
			if isAddrInUse(err) {
				return status.Error(codes.AlreadyExists, fmt.Sprintf("port %d is in use", newPort))
			}
			return rpcError(err)
		}
		// The port is free on this address. Give it straight back before the next.
		_ = probe.Close()
	}
	return nil
}

// isAddrInUse distinguishes "something is listening here" from every other reason a
// listen can fail, so a permission problem is reported as a permission problem rather
// than as a port conflict.
func isAddrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return errors.Is(opErr.Err, syscall.EADDRINUSE)
	}
	// Fall back to the text for platforms whose error wrapping differs.
	return strings.Contains(strings.ToLower(err.Error()), "address already in use")
}
