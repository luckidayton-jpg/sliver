package evilginx

/*
	Sliver C2 bridge for the evilginx platform.

	This package is the ONLY coupling point between the evilginx core and the
	embedded Sliver server (mirroring gophish/evilginx/bridge.go). Everything the
	platform needs from Sliver is exposed through SliverBridge below.

	Design:
	  * The Sliver server is started embedded (daemon mode) via the same
	    bootstrap sequence sliver-server uses (assets -> CAs -> crypto ->
	    default C2 profiles -> mTLS operator listener).
	  * A scoped operator account ("xavier") is minted via
	    console.NewOperatorConfig and the bridge dials back over mTLS using the
	    plain-rpcpb client (no console). This keeps the operator-facing RPC
	    surface intact for feature parity (sessions, beacons, listeners, loot,
	    websites, generate ...) without importing the interactive console.

	Update discipline: when rebasing sliver upstream, only this file (and any
	feature-specific files in this package) must be reconciled with upstream
	signature changes.
*/

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/bishopfox/sliver/protobuf/commonpb"
	"github.com/bishopfox/sliver/protobuf/rpcpb"
	"github.com/bishopfox/sliver/server/assets"
	"github.com/bishopfox/sliver/server/c2"
	"github.com/bishopfox/sliver/server/certs"
	"github.com/bishopfox/sliver/server/configs"
	"github.com/bishopfox/sliver/server/console"
	"github.com/bishopfox/sliver/server/cryptography"
	servertransport "github.com/bishopfox/sliver/server/transport"
)

const (
	OperatorName = "xavier"

	defaultListenPort = uint16(31337)
	grpcConnectTimeout = 10 * time.Second
	grpcMaxMessageSize = 2 * 1024 * 1024 * 1024 // 2Gb - 1
)

// SliverConfig - options for starting the embedded Sliver server.
type SliverConfig struct {
	Host  string
	Port  uint16
	AppDir string // isolated SLIVER_APP_DIR (must be absolute)
}

// SliverBridge - singleton broker between the evilginx platform and Sliver.
type SliverBridge struct {
	config *SliverConfig

	grpcServer  *grpc.Server
	listener    net.Listener
	client      rpcpb.SliverRPCClient
	conn        *grpc.ClientConn
	operatorCfg *clientConfig
}

// clientConfig mirrors client/assets.ClientConfig so the bridge never imports
// client packages (server layering rule). JSON-compatible field-for-field.
type clientConfig struct {
	Operator      string `json:"operator"`
	LHost         string `json:"lhost"`
	LPort         int    `json:"lport"`
	Token         string `json:"token"`
	CACertificate string `json:"ca_certificate"`
	PrivateKey    string `json:"private_key"`
	Certificate   string `json:"certificate"`
}

type tokenAuth struct {
	token string
}

func (t tokenAuth) GetRequestMetadata(ctx context.Context, in ...string) (map[string]string, error) {
	return map[string]string{"Authorization": "Bearer " + t.token}, nil
}

func (tokenAuth) RequireTransportSecurity() bool { return true }

// NewBridge - construct a bridge, no side effects until Start().
func NewBridge(cfg *SliverConfig) *SliverBridge {
	if cfg == nil {
		cfg = &SliverConfig{}
	}
	if cfg.Port == 0 {
		cfg.Port = defaultListenPort
	}
	return &SliverBridge{config: cfg}
}

// Start - bootstrap the embedded Sliver server and dial it over mTLS.
func (s *SliverBridge) Start() error {
	if s.listener != nil {
		return fmt.Errorf("sliver bridge: already started")
	}

	if s.config.AppDir != "" {
		if err := os.Setenv("SLIVER_APP_DIR", s.config.AppDir); err != nil {
			return fmt.Errorf("sliver bridge: %w", err)
		}
	}

	assets.Setup(false, false)
	certs.SetupCAs()
	certs.SetupWGKeys()
	cryptography.AgeServerKeyPair()
	cryptography.MinisignServerPrivateKey()
	c2.SetupDefaultC2Profiles()
	_, _ = configs.LoadCrackConfig()

	port := s.config.Port
	grpcServer, ln, err := servertransport.StartMtlsClientListener(s.config.Host, port)
	if err != nil {
		return fmt.Errorf("sliver bridge: start mtls listener: %w", err)
	}
	s.grpcServer = grpcServer
	s.listener = ln

	operatorJSON, err := console.NewOperatorConfig(OperatorName, s.config.Host, port, []string{"all"}, false)
	if err != nil {
		s.Stop()
		return fmt.Errorf("sliver bridge: mint operator config: %w", err)
	}

	cfg := &clientConfig{}
	if err := json.Unmarshal(operatorJSON, cfg); err != nil {
		s.Stop()
		return fmt.Errorf("sliver bridge: parse operator config: %w", err)
	}
	if cfg.LHost == "" {
		cfg.LHost = s.config.Host
	}
	if cfg.LPort == 0 {
		cfg.LPort = int(port)
	}
	s.operatorCfg = cfg

	if err := s.dial(); err != nil {
		s.Stop()
		return err
	}
	return nil
}

func (s *SliverBridge) dial() error {
	addr := net.JoinHostPort(s.operatorCfg.LHost, fmt.Sprintf("%d", s.operatorCfg.LPort))
	caPool := certPool(s.operatorCfg.CACertificate)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{mustTLSCert(s.operatorCfg.Certificate, s.operatorCfg.PrivateKey)},
		RootCAs:       caPool,
		MinVersion:    tls.VersionTLS12,
		// Sliver operator certs are minted with opaque identities and no IP
		// SANs; the console client pins the CA chain instead of the hostname
		// (root-only verification). We mirror that here.
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			return verifyServerChain(caPool, rawCerts)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), grpcConnectTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithPerRPCCredentials(tokenAuth{token: s.operatorCfg.Token}),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(grpcMaxMessageSize)),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("sliver bridge: dial %s: %w", addr, err)
	}
	s.conn = conn
	s.client = rpcpb.NewSliverRPCClient(conn)
	return nil
}

// Stop - stop the gRPC listener and close the client, in either order safely.
func (s *SliverBridge) Stop() {
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	if s.grpcServer != nil {
		s.grpcServer.Stop()
		s.grpcServer = nil
	}
	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
	s.client = nil
}

// Running - true when the embedded server + client connection are up.
func (s *SliverBridge) Running() bool {
	return s.client != nil && s.listener != nil
}

// Status - quick health snapshot for the web console.
type Status struct {
	Running    bool   `json:"running"`
	Operator   string `json:"operator"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	AppDir     string `json:"app_dir"`
	Sessions   int    `json:"sessions"`
	Jobs       int    `json:"jobs"`
	Version    string `json:"version"`
	Error      string `json:"error,omitempty"`
}

func (s *SliverBridge) Status() *Status {
	st := &Status{
		Running:  s.Running(),
		Operator: OperatorName,
		Host:     s.config.Host,
		Port:     int(s.config.Port),
		AppDir:   s.config.AppDir,
	}
	if !s.Running() {
		return st
	}

	sessions, err := s.client.GetSessions(context.Background(), &commonpb.Empty{})
	if err != nil {
		st.Error = err.Error()
		return st
	}
	jobs, err := s.client.GetJobs(context.Background(), &commonpb.Empty{})
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.Sessions = len(sessions.Sessions)
	if jobs != nil && jobs.Active != nil {
		st.Jobs = len(jobs.Active)
	}
	return st
}

// ListSessions - summarize active sessions for the console tables.
func (s *SliverBridge) ListSessions() ([]SessionSummary, error) {
	if !s.Running() {
		return nil, fmt.Errorf("sliver not running")
	}
	resp, err := s.client.GetSessions(context.Background(), &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	out := make([]SessionSummary, 0, len(resp.Sessions))
	for _, sess := range resp.Sessions {
		out = append(out, SessionSummary{
			ID:            sess.ID,
			Name:          sess.Name,
			Transport:     sess.Transport,
			RemoteAddress: sess.RemoteAddress,
			Hostname:      sess.Hostname,
			Username:      sess.Username,
			OS:            sess.OS,
			Arch:          sess.Arch,
			PID:           sess.PID,
			LastCheckin:   sess.LastCheckin,
			ActiveC2:      sess.ActiveC2,
		})
	}
	return out, nil
}

// ListJobs - summarize listeners/jobs for the console tables.
func (s *SliverBridge) ListJobs() ([]JobSummary, error) {
	if !s.Running() {
		return nil, fmt.Errorf("sliver not running")
	}
	resp, err := s.client.GetJobs(context.Background(), &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	out := []JobSummary{}
	for _, job := range resp.Active {
		out = append(out, JobSummary{
			ID:       job.ID,
			Name:     job.Name,
			Protocol: job.Protocol,
			Port:     int(job.Port),
		})
	}
	return out, nil
}

// --- typed summaries (stable JSON surface for the evilginx webapi) ---

type SessionSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Transport     string `json:"transport"`
	RemoteAddress string `json:"remote_address"`
	Hostname      string `json:"hostname"`
	Username      string `json:"username"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	PID           int32  `json:"pid"`
	LastCheckin   int64  `json:"last_checkin"`
	ActiveC2      string `json:"active_c2,omitempty"`
}

type JobSummary struct {
	ID       uint32 `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

// --- TLS helpers (no client imports; root-only verification like sliver) ---

func mustTLSCert(certPEM, keyPEM string) tls.Certificate {
	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		panic(fmt.Sprintf("invalid client cert: %v", err))
	}
	return cert
}

func certPool(caPEM string) *x509.CertPool {
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM([]byte(caPEM)); !ok {
		panic("invalid CA pem")
	}
	return pool
}

// verifyServerChain - accept only a peer certificate that chains (chain-of-trust
// to our pinned CA). Hostname identity is deliberately not checked: Sliver
// operator certificates do not carry IP SANs and are pinned by CA fingerprint.
func verifyServerChain(caPool *x509.CertPool, rawCerts [][]byte) error {
	if len(rawCerts) == 0 {
		return fmt.Errorf("no server certificate presented")
	}
	leaf, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return fmt.Errorf("parse server cert: %w", err)
	}

	intermediates := x509.NewCertPool()
	for _, raw := range rawCerts[1:] {
		if cert, err := x509.ParseCertificate(raw); err == nil {
			intermediates.AddCert(cert)
		}
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         caPool,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		return fmt.Errorf("verify server chain: %w", err)
	}
	return nil
}