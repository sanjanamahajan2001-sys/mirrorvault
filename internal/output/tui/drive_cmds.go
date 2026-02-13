package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"mirrorvault/internal/drive"

	tea "github.com/charmbracelet/bubbletea"
)

type driveDeviceCodeMsg struct {
	ConnectID int
	Code      *drive.DeviceCode
	Err       error
}

type driveConnectSuccessMsg struct {
	ConnectID       int
	Config          *drive.Config
	AccountEmail    string
	AccountRemaining int64
	AccountTotal     int64
	Err             error
}

type driveFoldersMsg struct {
	Folders          []drive.FolderItem
	AccountRemaining int64
	AccountTotal     int64
	Err              error
}

type driveFolderSizeMsg struct {
	FolderID string
	Size     int64
	Err      error
}

type driveFilesMsg struct {
	Files []drive.FileItem
	Err   error
}

type driveCreateFolderMsg struct {
	Folder *drive.FolderItem
	Err    error
}

type driveDownloadMsg struct {
	Path string
	Err  error
}

type driveBrowserAuthMsg struct {
	ConnectID int
	Session   *drive.BrowserSession
	Err       error
}

type driveConfigLoadedMsg struct {
	Config *drive.Config
	Err    error
}

func resolveDriveClientCreds(m TUIModel) (string, string) {
	clientID := os.Getenv("MV_GDRIVE_CLIENT_ID")
	clientSecret := os.Getenv("MV_GDRIVE_CLIENT_SECRET")
	if clientID == "" && m.DriveConfig != nil {
		clientID = m.DriveConfig.ClientID
	}
	if clientSecret == "" && m.DriveConfig != nil {
		clientSecret = m.DriveConfig.ClientSecret
	}
	return clientID, clientSecret
}

func detectRedirectHost() (string, string, bool) {
	if override := os.Getenv("MV_GDRIVE_REDIRECT_HOST"); override != "" {
		return override, "redirect host set via MV_GDRIVE_REDIRECT_HOST", false
	}
	// For production-grade browser flow, use loopback. Google blocks private IPs
	// without device_id/device_name, so avoid private IP redirects.
	if isWSL() {
		return "localhost", "detected WSL: using localhost with bind-all listener", true
	}
	return "localhost", "using localhost for loopback callback", false
}

func isWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err == nil && strings.Contains(strings.ToLower(string(data)), "microsoft") {
		return true
	}
	data, err = os.ReadFile("/proc/version")
	if err == nil && strings.Contains(strings.ToLower(string(data)), "microsoft") {
		return true
	}
	return false
}

// Deprecated helpers removed: private IP redirects are not used for browser flow.

func (m TUIModel) driveStartBrowserAuthCmd(connectID int) tea.Cmd {
	return func() tea.Msg {
		clientID, clientSecret := resolveDriveClientCreds(m)
		if clientID == "" || clientSecret == "" {
			return driveBrowserAuthMsg{
				ConnectID: connectID,
				Err:       fmt.Errorf("google drive client id or secret not set"),
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		redirectHost, _, bindAll := detectRedirectHost()
	session, err := drive.StartBrowserAuthSession(ctx, clientID, clientSecret, []string{
			"https://www.googleapis.com/auth/drive",
			"https://www.googleapis.com/auth/userinfo.email",
		}, redirectHost, bindAll)
		return driveBrowserAuthMsg{ConnectID: connectID, Session: session, Err: err}
	}
}

func (m TUIModel) driveBrowserWaitCmd(connectID int, session *drive.BrowserSession) tea.Cmd {
	return func() tea.Msg {
		if session == nil {
			return driveConnectSuccessMsg{ConnectID: connectID, Err: fmt.Errorf("browser session not initialized")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		select {
		case code := <-session.CodeCh:
			token, err := drive.ExchangeBrowserCode(ctx, session, code)
			if err != nil {
				return driveConnectSuccessMsg{ConnectID: connectID, Err: err}
			}
			if token.RefreshToken == "" {
				return driveConnectSuccessMsg{ConnectID: connectID, Err: fmt.Errorf("refresh token not returned; re-run with consent")}
			}
			email, _ := drive.FetchAccountEmail(ctx, token.AccessToken)
			clientID, clientSecret := resolveDriveClientCreds(m)
			cfg := &drive.Config{
				Enabled:      true,
				Provider:     "google_drive",
				AccountEmail: email,
				ClientID:     clientID,
				ClientSecret: clientSecret,
				RefreshToken: token.RefreshToken,
				TokenURI:     "https://oauth2.googleapis.com/token",
				AuthMethod:   "browser",
				Scope:        "https://www.googleapis.com/auth/drive",
			}
			if err := drive.SaveConfig(cfg); err != nil {
				return driveConnectSuccessMsg{ConnectID: connectID, Err: err}
			}
			client, err := drive.NewClient(ctx, cfg)
			if err != nil {
				return driveConnectSuccessMsg{ConnectID: connectID, Config: cfg, AccountEmail: email}
			}
			quota, err := drive.GetAccountQuota(ctx, client.Service)
			if err != nil {
				return driveConnectSuccessMsg{ConnectID: connectID, Config: cfg, AccountEmail: email}
			}
			return driveConnectSuccessMsg{
				ConnectID:        connectID,
				Config:           cfg,
				AccountEmail:     email,
				AccountRemaining: quota.Remaining,
				AccountTotal:     quota.Limit,
			}
		case err := <-session.ErrCh:
			return driveConnectSuccessMsg{ConnectID: connectID, Err: err}
		case <-ctx.Done():
			return driveConnectSuccessMsg{ConnectID: connectID, Err: fmt.Errorf("browser authorization timed out")}
		}
	}
}

func (m TUIModel) driveSetupInitCmd() tea.Cmd {
	return m.driveReloadConfigCmd()
}

func (m TUIModel) driveReloadConfigCmd() tea.Cmd {
	return func() tea.Msg {
		cfg, err := drive.LoadConfig()
		if cfg == nil {
			cfg = &drive.Config{Provider: "google_drive"}
		}
		return driveConfigLoadedMsg{Config: cfg, Err: err}
	}
}

func (m TUIModel) driveStartDeviceFlowCmd(connectID int) tea.Cmd {
	return func() tea.Msg {
		clientID, _ := resolveDriveClientCreds(m)
		if clientID == "" {
			return driveDeviceCodeMsg{
				ConnectID: connectID,
				Err:       fmt.Errorf("google drive client id not set"),
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		code, err := drive.StartDeviceFlow(ctx, clientID, []string{
			"https://www.googleapis.com/auth/drive.file",
			"https://www.googleapis.com/auth/userinfo.email",
		})
		return driveDeviceCodeMsg{ConnectID: connectID, Code: code, Err: err}
	}
}

func (m TUIModel) drivePollTokenCmd(connectID int, device *drive.DeviceCode) tea.Cmd {
	return func() tea.Msg {
		clientID, clientSecret := resolveDriveClientCreds(m)
		if clientID == "" || clientSecret == "" {
			return driveConnectSuccessMsg{
				ConnectID: connectID,
				Err:       fmt.Errorf("google drive client id or secret not set"),
			}
		}
		ctx := context.Background()
		token, err := drive.PollForToken(ctx, clientID, clientSecret, device)
		if err != nil {
			return driveConnectSuccessMsg{ConnectID: connectID, Err: err}
		}
		if token.RefreshToken == "" {
			return driveConnectSuccessMsg{
				ConnectID: connectID,
				Err:       fmt.Errorf("refresh token not returned; revoke access and try again"),
			}
		}
		email, _ := drive.FetchAccountEmail(ctx, token.AccessToken)

		cfg := &drive.Config{
			Enabled:      true,
			Provider:     "google_drive",
			AccountEmail: email,
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RefreshToken: token.RefreshToken,
			TokenURI:     "https://oauth2.googleapis.com/token",
			AuthMethod:   "device",
			Scope:        "https://www.googleapis.com/auth/drive.file",
		}
		if err := drive.SaveConfig(cfg); err != nil {
			return driveConnectSuccessMsg{ConnectID: connectID, Err: err}
		}

		client, err := drive.NewClient(ctx, cfg)
		if err != nil {
			return driveConnectSuccessMsg{ConnectID: connectID, Err: err}
		}
		quota, err := drive.GetAccountQuota(ctx, client.Service)
		if err != nil {
			return driveConnectSuccessMsg{ConnectID: connectID, Config: cfg, AccountEmail: email}
		}

		return driveConnectSuccessMsg{
			ConnectID:        connectID,
			Config:           cfg,
			AccountEmail:     email,
			AccountRemaining: quota.Remaining,
			AccountTotal:     quota.Limit,
		}
	}
}

func (m TUIModel) driveAccountAndFoldersCmd() tea.Cmd {
	return func() tea.Msg {
		if m.DriveConfig == nil {
			return driveFoldersMsg{Err: fmt.Errorf("drive config not available")}
		}
		client, err := drive.NewClient(context.Background(), m.DriveConfig)
		if err != nil {
			return driveFoldersMsg{Err: err}
		}
		folders, err := drive.ListFolders(context.Background(), client.Service, "root")
		if err != nil {
			return driveFoldersMsg{Err: err}
		}
		quota, err := drive.GetAccountQuota(context.Background(), client.Service)
		if err != nil {
			return driveFoldersMsg{Folders: folders, Err: nil}
		}
		return driveFoldersMsg{
			Folders:          folders,
			AccountRemaining: quota.Remaining,
			AccountTotal:     quota.Limit,
		}
	}
}

func (m TUIModel) driveFolderSizeCmd(folderID string) tea.Cmd {
	return func() tea.Msg {
		if m.DriveConfig == nil {
			return driveFolderSizeMsg{FolderID: folderID, Err: fmt.Errorf("drive config not available")}
		}
		client, err := drive.NewClient(context.Background(), m.DriveConfig)
		if err != nil {
			return driveFolderSizeMsg{FolderID: folderID, Err: err}
		}
		size, err := drive.FolderSize(context.Background(), client.Service, folderID)
		return driveFolderSizeMsg{FolderID: folderID, Size: size, Err: err}
	}
}

func (m TUIModel) driveCreateFolderCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if m.DriveConfig == nil {
			return driveCreateFolderMsg{Err: fmt.Errorf("drive config not available")}
		}
		client, err := drive.NewClient(context.Background(), m.DriveConfig)
		if err != nil {
			return driveCreateFolderMsg{Err: err}
		}
		folder, err := drive.CreateFolder(context.Background(), client.Service, name, "root")
		return driveCreateFolderMsg{Folder: folder, Err: err}
	}
}

func (m TUIModel) driveListFilesCmd() tea.Cmd {
	return func() tea.Msg {
		if m.DriveConfig == nil || m.DriveConfig.FolderID == "" {
			return driveFilesMsg{Err: fmt.Errorf("drive folder not configured")}
		}
		client, err := drive.NewClient(context.Background(), m.DriveConfig)
		if err != nil {
			return driveFilesMsg{Err: err}
		}
		files, err := drive.ListFiles(context.Background(), client.Service, m.DriveConfig.FolderID)
		return driveFilesMsg{Files: files, Err: err}
	}
}

func (m TUIModel) driveDownloadCmd(file drive.FileItem) tea.Cmd {
	return func() tea.Msg {
		if m.DriveConfig == nil {
			return driveDownloadMsg{Err: fmt.Errorf("drive config not available")}
		}
		client, err := drive.NewClient(context.Background(), m.DriveConfig)
		if err != nil {
			return driveDownloadMsg{Err: err}
		}
		destDir := filepath.Join(os.TempDir(), "mirrorvault-drive")
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return driveDownloadMsg{Err: err}
		}
		destPath := filepath.Join(destDir, file.Name)
		if err := drive.DownloadFile(context.Background(), client.Service, file.ID, destPath); err != nil {
			return driveDownloadMsg{Err: err}
		}
		return driveDownloadMsg{Path: destPath}
	}
}
