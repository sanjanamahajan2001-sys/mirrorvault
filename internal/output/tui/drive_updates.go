package tui

import (
	"fmt"
	"strings"

	"mirrorvault/internal/drive"

	tea "github.com/charmbracelet/bubbletea"
)

func (m TUIModel) updateDriveMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case driveBrowserAuthMsg:
		if msg.ConnectID != m.DriveConnectID {
			return m, nil, true
		}
		if msg.Err != nil {
			m.DriveConnectError = msg.Err
			m.DriveConnectMessage = "Browser authorization failed to start"
			return m, nil, true
		}
		m.DriveBrowserSession = msg.Session
		m.DriveBrowserAuthURL = ""
		if msg.Session != nil {
			m.DriveBrowserAuthURL = msg.Session.AuthURL
		}
		if msg.Session != nil && msg.Session.RedirectHost != "" {
			m.DriveConnectMessage = fmt.Sprintf("Waiting for browser approval... (callback: %s, listen: %s)", msg.Session.RedirectHost, msg.Session.ListenerHost)
		} else {
			m.DriveConnectMessage = "Waiting for browser approval..."
		}
		return m, m.driveBrowserWaitCmd(m.DriveConnectID, msg.Session), true

	case driveConfigLoadedMsg:
		m.DriveConfig = msg.Config
		m.DriveConfigLoadError = msg.Err
		m.DriveEnabled = m.DriveConfig != nil && m.DriveConfig.Enabled && m.DriveConfig.IsConfigured()
		if m.DriveEnabled {
			return m, m.driveAccountAndFoldersCmd(), true
		}
		return m, nil, true

	case driveDeviceCodeMsg:
		if msg.ConnectID != m.DriveConnectID {
			return m, nil, true
		}
		if msg.Err != nil {
			m.DriveConnectError = msg.Err
			m.DriveConnectMessage = "Failed to start device authorization"
			m.DriveConnectInProgress = false
			return m, nil, true
		}
		m.DriveDeviceCode = msg.Code.DeviceCode
		m.DriveUserCode = msg.Code.UserCode
		m.DriveVerificationURL = msg.Code.VerificationURL
		m.DriveConnectMessage = "Waiting for approval..."
		return m, m.drivePollTokenCmd(m.DriveConnectID, msg.Code), true

	case driveConnectSuccessMsg:
		if msg.ConnectID != m.DriveConnectID {
			return m, nil, true
		}
		m.DriveConnectInProgress = false
		if m.DriveBrowserSession != nil && m.DriveBrowserSession.Shutdown != nil {
			m.DriveBrowserSession.Shutdown()
			m.DriveBrowserSession = nil
		}
		if msg.Err != nil {
			errText := strings.ToLower(msg.Err.Error())
			if m.DriveConnectMethod == "browser" &&
				(strings.Contains(errText, "connection refused") ||
					strings.Contains(errText, "refused to connect") ||
					strings.Contains(errText, "timed out")) {
				m.DriveConnectMethod = "device"
				m.DriveConnectInProgress = true
				m.DriveConnectError = msg.Err
				m.DriveConnectMessage = "Browser callback failed; switching to Device flow (requires TV and Limited Input client)"
				m.DriveUserCode = ""
				m.DriveVerificationURL = ""
				m.DriveDeviceCode = ""
				m.DriveBrowserAuthURL = ""
				return m, m.driveStartDeviceFlowCmd(m.DriveConnectID), true
			}
			m.DriveConnectError = msg.Err
			m.DriveConnectMessage = "Drive connection failed"
			return m, nil, true
		}
		m.DriveConnectError = nil
		m.DriveConnectMessage = "Connected securely to Google Drive"
		m.DriveConfig = msg.Config
		m.DriveEnabled = true
		if msg.AccountEmail != "" {
			m.DriveConfig.AccountEmail = msg.AccountEmail
		}
		m.DriveAccountRemaining = msg.AccountRemaining
		m.DriveAccountTotal = msg.AccountTotal
		m.ViewState = ViewDriveFolderSelect
		m.DriveFolderIndex = 0
		return m, m.driveAccountAndFoldersCmd(), true

	case driveFoldersMsg:
		if msg.Err != nil {
			m.DriveFolderError = msg.Err
			return m, nil, true
		}
		m.DriveFolders = msg.Folders
		m.DriveFolderError = nil
		if msg.AccountTotal > 0 {
			m.DriveAccountRemaining = msg.AccountRemaining
			m.DriveAccountTotal = msg.AccountTotal
		}
		m.DriveFolderIndex = 0
		m.DriveFolderSize = 0
		m.DriveFolderSizeLoading = false
		if m.DriveConfig != nil && m.DriveConfig.FolderID != "" {
			for i, folder := range m.DriveFolders {
				if folder.ID == m.DriveConfig.FolderID {
					m.DriveFolderIndex = i + 1
					m.DriveFolderSizeLoading = true
					m.DriveFolderSize = 0
					return m, m.driveFolderSizeForCurrent(), true
				}
			}
		}
		return m, nil, true

	case driveFolderSizeMsg:
		folder := m.driveFolderFromIndex()
		if folder == nil || folder.ID != msg.FolderID {
			return m, nil, true
		}
		m.DriveFolderSizeLoading = false
		if msg.Err != nil {
			m.DriveFolderError = msg.Err
			return m, nil, true
		}
		m.DriveFolderSize = msg.Size
		m.DriveFolderError = nil
		return m, nil, true

	case driveCreateFolderMsg:
		if msg.Err != nil {
			m.DriveFolderError = msg.Err
			return m, nil, true
		}
		if msg.Folder != nil && m.DriveConfig != nil {
			m.DriveConfig.FolderID = msg.Folder.ID
			m.DriveConfig.FolderName = msg.Folder.Name
			m.DriveConfig.Enabled = true
			m.DriveEnabled = true
			_ = saveDriveConfig(m)
			m.ViewState = ViewSelectEngine
			return m, nil, true
		}
		return m, nil, true

	case driveFilesMsg:
		if msg.Err != nil {
			m.DriveDownloadError = msg.Err
			return m, nil, true
		}
		m.DriveFiles = msg.Files
		m.DriveFileIndex = 0
		m.DriveDownloadError = nil
		return m, nil, true

	case driveDownloadMsg:
		m.DriveDownloadInProgress = false
		if msg.Err != nil {
			m.DriveDownloadError = msg.Err
			m.ViewState = ViewDriveFileSelect
			return m, nil, true
		}
		m.DriveDownloadPath = msg.Path
		restorePath := msg.Path
		if strings.HasSuffix(strings.ToLower(msg.Path), ".tar.gz") || strings.HasSuffix(strings.ToLower(msg.Path), ".tgz") {
			extracted, err := drive.ExtractTarGz(msg.Path)
			if err != nil {
				m.DriveDownloadError = fmt.Errorf("failed to extract archive: %v", err)
				m.ViewState = ViewDriveFileSelect
				return m, nil, true
			}
			restorePath = extracted
		}
		m.RestoreDumpPath = restorePath
		if err := prepareRestorePlan(&m); err != nil {
			m.DriveDownloadError = fmt.Errorf("failed to prepare restore: %v", err)
			m.ViewState = ViewDriveFileSelect
			return m, nil, true
		}
		m.ViewState = ViewRestoreConfirm
		return m, nil, true
	}

	return m, nil, false
}
