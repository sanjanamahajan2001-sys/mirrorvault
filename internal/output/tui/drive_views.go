package tui

import (
	"fmt"
	"strings"
	"os"
	"time"

	"mirrorvault/internal/drive"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/aymanbagabas/go-osc52/v2"
)

func (m TUIModel) viewDriveSetup() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Google Drive Backup Setup") + "\n\n")

	if m.DriveConfig == nil || !m.DriveConfig.IsConfigured() {
		b.WriteString(AuthStyle.Render("Status: Not connected") + "\n\n")
		if m.DriveConfigLoadError != nil {
			warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
			b.WriteString(warnStyle.Render("Saved Drive connection could not be loaded.") + "\n")
			b.WriteString(warnStyle.Render("Reason: "+m.DriveConfigLoadError.Error()) + "\n")
			b.WriteString(warnStyle.Render("Fix: run MirrorVault as the same user that connected,") + "\n")
			b.WriteString(warnStyle.Render("or update permissions on /var/lib/mirrorvault/drive_config.json") + "\n\n")
		} else if m.DriveConfig != nil && m.DriveConfig.Loaded {
			warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
			source := m.DriveConfig.SourcePath
			if source == "" {
				source = "saved config"
			}
			b.WriteString(warnStyle.Render("Found saved Drive config but it is not authenticated.") + "\n")
			b.WriteString(warnStyle.Render("Source: "+source) + "\n")
			b.WriteString(warnStyle.Render("Fix: reconnect to refresh tokens.") + "\n\n")
		} else {
			warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
			b.WriteString(warnStyle.Render("No saved Drive config found yet.") + "\n")
			b.WriteString(warnStyle.Render("Checked: /var/lib/mirrorvault/drive_config.json") + "\n")
			b.WriteString(warnStyle.Render("Checked: ~/.config/mirrorvault/drive_config.json") + "\n\n")
		}
		b.WriteString("Why connect Drive?\n")
		b.WriteString("  • Keep a second copy of every manual backup\n")
		b.WriteString("  • Upload happens automatically after local backup\n")
		b.WriteString("  • Restore directly from Drive when needed\n\n")
	b.WriteString("Quick start:\n")
	b.WriteString("  1) Press I to enter Client ID + Secret\n")
	b.WriteString("  2) Press C to connect and choose method\n")
	b.WriteString("  3) Pick a backup folder\n\n")
	b.WriteString("Note: Drive access is limited to files/folders created by MirrorVault.\n\n")
		b.WriteString(FooterStyle.Render(" C connect • I enter client id/secret • Enter skip • Esc back "))
		return b.String()
	}

	status := "disabled"
	if m.DriveEnabled {
		status = "enabled"
	}
	b.WriteString(NoAuthStyle.Render(fmt.Sprintf("Status: %s", status)) + "\n")
	if m.DriveConfig.AccountEmail != "" {
		b.WriteString(fmt.Sprintf("Account: %s\n", m.DriveConfig.AccountEmail))
	}
	if m.DriveConfig.FolderName != "" {
		b.WriteString(fmt.Sprintf("Folder: %s\n", m.DriveConfig.FolderName))
	} else if m.DriveConfig.FolderID != "" {
		b.WriteString(fmt.Sprintf("Folder ID: %s\n", m.DriveConfig.FolderID))
	}
	if m.DriveConfig.SourcePath != "" {
		b.WriteString(fmt.Sprintf("Config: %s\n", m.DriveConfig.SourcePath))
	}
	if m.DriveConfig.ConnectedAt != "" {
		b.WriteString(fmt.Sprintf("Connected: %s\n", formatConnectedAt(m.DriveConfig.ConnectedAt)))
	}
	if m.DriveConfig.AuthMethod != "" {
		b.WriteString(fmt.Sprintf("Auth method: %s\n", m.DriveConfig.AuthMethod))
	}
	if m.DriveAccountTotal > 0 {
		b.WriteString(fmt.Sprintf("Drive space: %s remaining of %s\n", formatBytes(m.DriveAccountRemaining), formatBytes(m.DriveAccountTotal)))
	}
	b.WriteString("\nThis connection is saved. You only need to reconnect if you disconnect or switch accounts.\n")
	b.WriteString("Tip: D disables uploads but keeps the connection. E re-enables uploads.\n")

	if m.DriveConnectMessage != "" {
		b.WriteString("\n" + ItemStyle.Render(m.DriveConnectMessage) + "\n")
	}
	if m.DriveConnectError != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		b.WriteString("\n" + errorStyle.Render("Error: "+m.DriveConnectError.Error()) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(FooterStyle.Render(" Enter continue • F folder • C reconnect • D disable • E enable • X disconnect • Esc back "))
	return b.String()
}

func (m TUIModel) updateDriveSetup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "c":
		m.ViewState = ViewDriveConnectMethod
		m.DriveConnectMethodIndex = 0
		return m, nil
	case "i":
		m.ViewState = ViewDriveClientSetup
		m.DriveClientFieldIndex = 0
		return m, nil
	case "f":
		if m.DriveConfig == nil || !m.DriveConfig.IsConfigured() {
			m.DriveConnectError = fmt.Errorf("connect Drive before choosing a folder")
			return m, nil
		}
		m.ViewState = ViewDriveFolderSelect
		m.DriveFolderIndex = 0
		return m, m.driveAccountAndFoldersCmd()
	case "d":
		if m.DriveConfig != nil {
			m.DriveEnabled = false
			m.DriveConfig.Enabled = false
			_ = saveDriveConfig(m)
			m.DriveConnectMessage = "Drive disabled for backups"
		}
		return m, nil
	case "x":
		if m.DriveConfig != nil {
			m.DriveEnabled = false
			m.DriveConfig.Enabled = false
			m.DriveConfig.RefreshToken = ""
			m.DriveConfig.TokenURI = ""
			m.DriveConfig.AccountEmail = ""
			m.DriveConfig.FolderID = ""
			m.DriveConfig.FolderName = ""
			m.DriveConfig.AuthMethod = ""
			m.DriveConfig.Scope = ""
			m.DriveConfig.ConnectedAt = ""
			_ = saveDriveConfig(m)
			m.DriveConnectMessage = "Drive disconnected. Tokens cleared; reconnect to use Drive."
		}
		return m, nil
	case "e":
		if m.DriveConfig != nil && m.DriveConfig.IsConfigured() {
			m.DriveEnabled = true
			m.DriveConfig.Enabled = true
			_ = saveDriveConfig(m)
			m.DriveConnectMessage = "Drive enabled for backups"
		}
		return m, nil
	case "enter":
		if m.DriveEnabled && m.DriveConfig != nil && m.DriveConfig.FolderID == "" {
			m.ViewState = ViewDriveFolderSelect
			m.DriveFolderIndex = 0
			return m, m.driveAccountAndFoldersCmd()
		}
		m.ViewState = ViewSelectEngine
		return m, nil
	case "esc":
		m.ViewState = ViewScan
		return m, nil
	}
	return m, nil
}

func (m TUIModel) viewDriveConnect() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Connect Google Drive") + "\n\n")

	if m.DriveUserCode != "" && m.DriveVerificationURL != "" {
		b.WriteString("Visit this URL and enter the code:\n\n")
		b.WriteString(renderWrappedURL(m, m.DriveVerificationURL))
		b.WriteString(ItemStyle.Render("  Code: "+m.DriveUserCode) + "\n\n")
		if m.DriveConnectMethod == "device" {
			b.WriteString(ItemStyle.Render("Device flow requires OAuth Client ID: TV and Limited Input.") + "\n")
			b.WriteString(ItemStyle.Render("Desktop client credentials will NOT work here.") + "\n\n")
		}
	} else if m.DriveBrowserAuthURL != "" {
		b.WriteString("Authorization URL (open in browser):\n\n")
		b.WriteString(renderWrappedURL(m, m.DriveBrowserAuthURL))
		b.WriteString("\n")
		if m.DriveBrowserSession != nil && m.DriveBrowserSession.RedirectURL != "" {
			b.WriteString("Redirect URL (automatic callback - do not open):\n")
			b.WriteString(ItemStyle.Render("  "+m.DriveBrowserSession.RedirectURL) + "\n")
		}
		b.WriteString(ItemStyle.Render("Use the Authorization URL above. The Redirect URL is handled automatically.") + "\n")
		b.WriteString(ItemStyle.Render("Recommended: Device flow (more reliable across servers).") + "\n")
		b.WriteString(ItemStyle.Render("If browser callback fails, switch to Device flow.") + "\n")
		b.WriteString("\n")
	} else {
		b.WriteString("Starting device authorization...\n\n")
	}

	if m.DriveConnectMessage != "" {
		b.WriteString(ItemStyle.Render(m.DriveConnectMessage) + "\n")
	}
	if m.DriveConfig != nil && m.DriveConfig.ClientID != "" {
		clientID := m.DriveConfig.ClientID
		if len(clientID) > 6 {
			clientID = clientID[len(clientID)-6:]
		}
		b.WriteString(ItemStyle.Render("Using Client ID: ..."+clientID) + "\n")
	}
	if m.DriveConnectError != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		b.WriteString("\n" + errorStyle.Render("Error: "+m.DriveConnectError.Error()) + "\n")
		if m.DriveConnectMethod == "device" {
			errText := strings.ToLower(m.DriveConnectError.Error())
			if strings.Contains(errText, "connection refused") ||
				strings.Contains(errText, "refused to connect") ||
				strings.Contains(errText, "timed out") {
				infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
				b.WriteString(infoStyle.Render("Auto-switched to Device flow after browser callback failed.") + "\n")
				b.WriteString(infoStyle.Render("Device flow requires OAuth Client ID: TV and Limited Input.") + "\n")
			}
		}
		b.WriteString("\nTroubleshooting checklist:\n")
		errText := strings.ToLower(m.DriveConnectError.Error())
		if strings.Contains(errText, "client id not set") || strings.Contains(errText, "client id or secret not set") {
			b.WriteString("  • No Client ID/Secret found in config or environment\n")
			if m.DriveConnectMethod == "device" {
				b.WriteString("  • Device flow requires OAuth Client ID: TV and Limited Input\n")
			} else {
				b.WriteString("  • Browser flow requires OAuth Client ID: Desktop (loopback)\n")
			}
			b.WriteString("  • Press I to enter credentials, then try again\n")
		} else if strings.Contains(errText, "device id and device name") {
			b.WriteString("  • Google blocks private IP redirects without device_id/device_name\n")
			b.WriteString("  • Use Device flow for servers, or Browser flow on a local desktop\n")
		} else if strings.Contains(errText, "connection refused") || strings.Contains(errText, "timed out") || strings.Contains(errText, "refused to connect") {
			b.WriteString("  • Browser could not reach the local callback server\n")
			b.WriteString("  • Open the URL on the same machine as MirrorVault\n")
			b.WriteString("  • Recommended: switch to Device flow (no local callback)\n")
		} else if strings.Contains(errText, "invalid device flow scope") || strings.Contains(errText, "invalid_scope") {
			b.WriteString("  • This OAuth client allows limited scopes for device flow\n")
			b.WriteString("  • Use Drive scope: https://www.googleapis.com/auth/drive.file\n")
			b.WriteString("  • Reconnect after updating the client or scopes\n")
		} else if strings.Contains(errText, "invalid client type") {
			b.WriteString("  • Client type mismatch for the selected method\n")
			if m.DriveConnectMethod == "browser" {
				b.WriteString("  • Create OAuth Client ID: Desktop (loopback)\n")
			} else {
				b.WriteString("  • Create OAuth Client ID: TV and Limited Input\n")
			}
			b.WriteString("  • Then re-enter Client ID/Secret and connect again\n")
		} else if strings.Contains(errText, "401") || strings.Contains(errText, "unauthorized") {
			b.WriteString("  • Client ID and Client Secret must match the same OAuth client\n")
			if m.DriveConnectMethod == "browser" {
				b.WriteString("  • Create OAuth Client ID: Desktop (loopback)\n")
			} else {
				b.WriteString("  • Create OAuth Client ID: TV and Limited Input\n")
			}
			b.WriteString("  • Drive API must be enabled in this project\n")
			b.WriteString("  • If consent screen is in Testing, add your account as Test User\n")
		} else {
			b.WriteString("  • Check Client ID and Client Secret are correct\n")
			b.WriteString("  • Confirm Drive API is enabled\n")
			b.WriteString("  • Verify consent screen settings and test users\n")
		}
		b.WriteString("\nSuggested fix:\n")
		if m.DriveConnectMethod == "device" {
			b.WriteString("  • Create OAuth Client ID: TV and Limited Input\n")
		} else if m.DriveConnectMethod == "browser" {
			b.WriteString("  • Create OAuth Client ID: Desktop (loopback)\n")
		} else {
			b.WriteString("  • Use Device (TV and Limited Input) for servers\n")
		}
		b.WriteString("\nTip: Press I to re-enter Client ID/Secret if needed.\n")
	}

	b.WriteString("\n" + FooterStyle.Render(" Y copy URL • I enter client id/secret • Esc back "))
	return b.String()
}

func (m TUIModel) updateDriveConnect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		urlToCopy := ""
		if m.DriveBrowserAuthURL != "" {
			urlToCopy = m.DriveBrowserAuthURL
		} else if m.DriveVerificationURL != "" {
			urlToCopy = m.DriveVerificationURL
		}
		if urlToCopy != "" {
			if err := copyToClipboard(urlToCopy); err != nil {
				m.DriveConnectError = fmt.Errorf("failed to copy URL: %v", err)
			} else {
				m.DriveConnectMessage = "URL copied to clipboard"
			}
		}
		return m, nil
	case "i":
		m.ViewState = ViewDriveClientSetup
		m.DriveClientFieldIndex = 0
		return m, nil
	case "esc":
		m.DriveConnectID++
		m.DriveConnectInProgress = false
		m.ViewState = ViewDriveSetup
		return m, nil
	}
	return m, nil
}

func renderWrappedURL(m TUIModel, url string) string {
	width := m.TerminalWidth
	if width <= 0 {
		width = 80
	}
	indent := "  "
	maxLine := width - len(indent)
	if maxLine < 20 {
		maxLine = 20
	}
	var b strings.Builder
	remaining := url
	for len(remaining) > 0 {
		if len(remaining) <= maxLine {
			b.WriteString(ItemStyle.Render(indent+remaining) + "\n")
			break
		}
		b.WriteString(ItemStyle.Render(indent+remaining[:maxLine]) + "\n")
		remaining = remaining[maxLine:]
	}
	return b.String()
}

func formatConnectedAt(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.Local().Format("Jan 02, 2006 15:04 MST")
}

func copyToClipboard(text string) error {
	if text == "" {
		return fmt.Errorf("no URL to copy")
	}
	seq := osc52.New(text)
	_, err := seq.WriteTo(os.Stdout)
	return err
}

func (m TUIModel) viewDriveConnectMethod() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Choose Connection Method") + "\n\n")
	options := []string{
		"Device flow (recommended for servers)",
		"Browser redirect (desktop only)",
	}
	for i, opt := range options {
		cursor := "  "
		if i == m.DriveConnectMethodIndex {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, opt))
	}
	b.WriteString("\nNotes:\n")
	b.WriteString("  • Device: OAuth client type must be TV and Limited Input\n")
	b.WriteString("  • Browser: OAuth client type must be Desktop (loopback)\n")
	b.WriteString("  • Device flow is most reliable across Linux servers\n")
	b.WriteString("  • Device flow is limited to Drive files created by MirrorVault\n")
	b.WriteString("\n" + FooterStyle.Render(" ↑/↓ move • Enter select • Esc back "))
	return b.String()
}

func (m TUIModel) updateDriveConnectMethod(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		if m.DriveConnectMethodIndex > 0 {
			m.DriveConnectMethodIndex--
		}
	case "down":
		if m.DriveConnectMethodIndex < 1 {
			m.DriveConnectMethodIndex++
		}
	case "enter":
		m.ViewState = ViewDriveConnect
		m.DriveConnectID++
		m.DriveConnectInProgress = true
		m.DriveConnectError = nil
		m.DriveConnectMessage = "Starting secure connection..."
		m.DriveUserCode = ""
		m.DriveVerificationURL = ""
		m.DriveDeviceCode = ""
		m.DriveBrowserAuthURL = ""
		if m.DriveConnectMethodIndex == 0 {
			m.DriveConnectMethod = "device"
			return m, m.driveStartDeviceFlowCmd(m.DriveConnectID)
		}
		m.DriveConnectMethod = "browser"
		return m, m.driveStartBrowserAuthCmd(m.DriveConnectID)
	case "esc":
		m.ViewState = ViewDriveSetup
	}
	return m, nil
}

func (m TUIModel) viewDriveClientSetup() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Enter Google Drive Client Credentials") + "\n\n")
b.WriteString("Get these from Google Cloud Console:\n")
b.WriteString("  APIs & Services → Credentials → Create OAuth Client ID\n")
	b.WriteString("  Device flow (recommended): TV and Limited Input\n")
	b.WriteString("  Browser flow (desktop only): Desktop (loopback)\n\n")

	idLine := m.DriveClientIDInput
	secretLine := m.DriveClientSecretInput
	if idLine == "" {
		idLine = "[Type Client ID here]"
	}
	if secretLine == "" {
		secretLine = "[Type Client Secret here]"
	} else {
		secretLine = strings.Repeat("*", len(m.DriveClientSecretInput))
	}

	cursor := "> "
	blank := "  "
	if m.DriveClientFieldIndex == 0 {
		b.WriteString(fmt.Sprintf("%sClient ID: %s\n", cursor, idLine))
		b.WriteString(fmt.Sprintf("%sClient Secret: %s\n", blank, secretLine))
	} else {
		b.WriteString(fmt.Sprintf("%sClient ID: %s\n", blank, idLine))
		b.WriteString(fmt.Sprintf("%sClient Secret: %s\n", cursor, secretLine))
	}
	if m.DriveConnectError != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		b.WriteString("\n" + errorStyle.Render("Error: "+m.DriveConnectError.Error()) + "\n")
	}

	b.WriteString("\n" + FooterStyle.Render(" Enter next/save • Tab/↑/↓ switch • Esc back "))
	return b.String()
}

func (m TUIModel) updateDriveClientSetup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "down":
		if m.DriveClientFieldIndex == 0 {
			m.DriveClientFieldIndex = 1
		} else {
			m.DriveClientFieldIndex = 0
		}
		return m, nil
	case "shift+tab", "up", "backtab":
		if m.DriveClientFieldIndex == 1 {
			m.DriveClientFieldIndex = 0
		} else {
			m.DriveClientFieldIndex = 1
		}
		return m, nil
	case "enter":
		if m.DriveClientFieldIndex == 0 {
			if strings.TrimSpace(m.DriveClientIDInput) == "" {
				m.DriveConnectError = fmt.Errorf("client id is required")
				return m, nil
			}
			m.DriveClientFieldIndex = 1
			return m, nil
		}
		if strings.TrimSpace(m.DriveClientIDInput) == "" || strings.TrimSpace(m.DriveClientSecretInput) == "" {
			m.DriveConnectError = fmt.Errorf("client id and secret are required")
			return m, nil
		}
		if m.DriveConfig == nil {
			m.DriveConfig = &drive.Config{Provider: "google_drive"}
		}
		m.DriveConfig.ClientID = strings.TrimSpace(m.DriveClientIDInput)
		m.DriveConfig.ClientSecret = strings.TrimSpace(m.DriveClientSecretInput)
		if err := saveDriveConfig(m); err != nil {
			m.DriveConnectError = fmt.Errorf("failed to save credentials: %v", err)
			return m, nil
		}
		m.DriveConnectMessage = "Client credentials saved. Press C to connect."
		m.ViewState = ViewDriveSetup
		return m, nil
	case "esc":
		m.ViewState = ViewDriveConnect
		return m, nil
	case "backspace":
		m.DriveConnectError = nil
		if m.DriveClientFieldIndex == 0 {
			if len(m.DriveClientIDInput) > 0 {
				m.DriveClientIDInput = m.DriveClientIDInput[:len(m.DriveClientIDInput)-1]
			}
		} else {
			if len(m.DriveClientSecretInput) > 0 {
				m.DriveClientSecretInput = m.DriveClientSecretInput[:len(m.DriveClientSecretInput)-1]
			}
		}
		return m, nil
	default:
		if len(msg.Runes) > 0 {
			m.DriveConnectError = nil
			if m.DriveClientFieldIndex == 0 {
				m.DriveClientIDInput += string(msg.Runes)
			} else {
				m.DriveClientSecretInput += string(msg.Runes)
			}
		}
	}
	return m, nil
}

func (m TUIModel) viewDriveFolderSelect() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Select Google Drive Folder") + "\n\n")

	if m.DriveAccountTotal > 0 {
		b.WriteString(fmt.Sprintf("Drive space: %s remaining of %s\n", formatBytes(m.DriveAccountRemaining), formatBytes(m.DriveAccountTotal)))
	}
	if m.DriveConnectMessage != "" {
		b.WriteString(ItemStyle.Render(m.DriveConnectMessage) + "\n")
	}
	if m.DriveFolderSizeLoading {
		b.WriteString("Selected folder size: calculating...\n")
	} else if m.DriveFolderSize > 0 {
		b.WriteString(fmt.Sprintf("Selected folder size: %s\n", formatBytes(m.DriveFolderSize)))
	}
	if m.DriveFolderError != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		b.WriteString(errorStyle.Render("Error: "+m.DriveFolderError.Error()) + "\n")
	}
	b.WriteString("\n")

	options := []string{"[+] Create new folder"}
	for _, f := range m.DriveFolders {
		options = append(options, f.Name)
	}
	if len(options) == 1 {
		options = append(options, "(No folders found)")
	}

	for i, name := range options {
		cursor := "  "
		if i == m.DriveFolderIndex {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, name))
	}

	b.WriteString("\n" + FooterStyle.Render(" ↑/↓ move • Enter select • R refresh • Esc back "))
	return b.String()
}

func (m TUIModel) updateDriveFolderSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	optionsCount := len(m.DriveFolders) + 1
	if optionsCount < 1 {
		optionsCount = 1
	}

	switch msg.String() {
	case "up":
		if m.DriveFolderIndex > 0 {
			m.DriveFolderIndex--
			if m.DriveFolderIndex == 0 {
				m.DriveFolderSizeLoading = false
				m.DriveFolderSize = 0
				return m, nil
			}
			m.DriveFolderSizeLoading = true
			m.DriveFolderSize = 0
			return m, m.driveFolderSizeForCurrent()
		}
	case "down":
		if m.DriveFolderIndex < optionsCount-1 {
			m.DriveFolderIndex++
			if m.DriveFolderIndex == 0 {
				m.DriveFolderSizeLoading = false
				m.DriveFolderSize = 0
				return m, nil
			}
			m.DriveFolderSizeLoading = true
			m.DriveFolderSize = 0
			return m, m.driveFolderSizeForCurrent()
		}
	case "r":
		return m, m.driveAccountAndFoldersCmd()
	case "enter":
		if m.DriveFolderIndex == 0 {
			m.ViewState = ViewDriveFolderCreate
			m.DriveNewFolderName = ""
			m.DriveFolderError = nil
			return m, nil
		}
		folder := m.driveFolderFromIndex()
		if folder != nil {
			m.DriveConfig.FolderID = folder.ID
			m.DriveConfig.FolderName = folder.Name
			m.DriveConfig.Enabled = true
			m.DriveEnabled = true
			_ = saveDriveConfig(m)
			m.ViewState = ViewSelectEngine
			return m, nil
		}
	case "esc":
		m.ViewState = ViewDriveSetup
		return m, nil
	}
	return m, nil
}

func (m TUIModel) viewDriveFolderCreate() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Create Google Drive Folder") + "\n\n")
	b.WriteString("Enter a folder name:\n\n")
	if m.DriveNewFolderName == "" {
		b.WriteString(ItemStyle.Render("  [Type folder name here]") + "\n")
	} else {
		b.WriteString(ItemStyle.Render("  "+m.DriveNewFolderName) + "\n")
	}
	if m.DriveFolderError != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		b.WriteString("\n" + errorStyle.Render("Error: "+m.DriveFolderError.Error()) + "\n")
	}
	b.WriteString("\n" + FooterStyle.Render(" Enter create • Esc back "))
	return b.String()
}

func (m TUIModel) updateDriveFolderCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if strings.TrimSpace(m.DriveNewFolderName) == "" {
			m.DriveFolderError = fmt.Errorf("folder name cannot be empty")
			return m, nil
		}
		m.DriveFolderError = nil
		return m, m.driveCreateFolderCmd(strings.TrimSpace(m.DriveNewFolderName))
	case "esc":
		m.ViewState = ViewDriveFolderSelect
		return m, nil
	case "backspace":
		if len(m.DriveNewFolderName) > 0 {
			m.DriveNewFolderName = m.DriveNewFolderName[:len(m.DriveNewFolderName)-1]
		}
		return m, nil
	default:
		if len(msg.Runes) > 0 {
			m.DriveNewFolderName += string(msg.Runes)
		}
	}
	return m, nil
}

func (m TUIModel) viewRestoreSource() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Select Restore Source") + "\n\n")
	options := []string{"Local file", "Google Drive"}
	for i, opt := range options {
		cursor := "  "
		if i == m.RestoreSourceIndex {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, opt))
	}
	if m.RestoreError != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		b.WriteString("\n" + errorStyle.Render("Error: "+m.RestoreError.Error()) + "\n")
	}
	b.WriteString("\n" + FooterStyle.Render(" ↑/↓ move • Enter select • Esc back "))
	return b.String()
}

func (m TUIModel) updateRestoreSource(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.RestoreError = nil
		if m.RestoreSourceIndex > 0 {
			m.RestoreSourceIndex--
		}
	case "down":
		m.RestoreError = nil
		if m.RestoreSourceIndex < 1 {
			m.RestoreSourceIndex++
		}
	case "enter":
		m.RestoreError = nil
		if m.RestoreSourceIndex == 0 {
			m.RestoreSource = "local"
			m.ViewState = ViewRestoreDumpPath
			return m, nil
		}
		m.RestoreSource = "drive"
		if m.DriveConfig == nil || !m.DriveConfig.IsConfigured() || m.DriveConfig.FolderID == "" {
			m.RestoreError = fmt.Errorf("google drive not connected or folder not set")
			m.ViewState = ViewRestoreSource
			return m, nil
		}
		m.ViewState = ViewDriveFileSelect
		m.DriveFileIndex = 0
		return m, m.driveListFilesCmd()
	case "esc":
		m.RestoreError = nil
		m.ViewState = ViewRestoreSelectDB
	}
	return m, nil
}

func (m TUIModel) viewDriveFileSelect() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Select Drive Backup File") + "\n\n")

	if m.DriveConfig != nil && m.DriveConfig.FolderName != "" {
		b.WriteString(fmt.Sprintf("Folder: %s\n\n", m.DriveConfig.FolderName))
	}
	if m.DriveDownloadError != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		b.WriteString(errorStyle.Render("Error: "+m.DriveDownloadError.Error()) + "\n\n")
	}

	if len(m.DriveFiles) == 0 {
		b.WriteString("No backup files found in Drive folder.\n")
		b.WriteString("\n" + FooterStyle.Render(" Esc back "))
		return b.String()
	}

	for i, file := range m.DriveFiles {
		cursor := "  "
		if i == m.DriveFileIndex {
			cursor = "> "
		}
		display := fmt.Sprintf("%s (%s)", file.Name, formatBytes(file.Size))
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, display))
	}
	b.WriteString("\n" + FooterStyle.Render(" ↑/↓ move • Enter download • Esc back "))
	return b.String()
}

func (m TUIModel) updateDriveFileSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		if m.DriveFileIndex > 0 {
			m.DriveFileIndex--
		}
	case "down":
		if m.DriveFileIndex < len(m.DriveFiles)-1 {
			m.DriveFileIndex++
		}
	case "enter":
		if m.DriveFileIndex >= 0 && m.DriveFileIndex < len(m.DriveFiles) {
			m.ViewState = ViewDriveDownload
			m.DriveDownloadInProgress = true
			m.DriveDownloadError = nil
			file := m.DriveFiles[m.DriveFileIndex]
			return m, m.driveDownloadCmd(file)
		}
	case "esc":
		m.ViewState = ViewRestoreSource
		return m, nil
	}
	return m, nil
}

func (m TUIModel) viewDriveDownload() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Downloading Backup from Drive") + "\n\n")
	if m.DriveDownloadInProgress {
		b.WriteString("Downloading selected backup file...\n")
	}
	if m.DriveDownloadError != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		b.WriteString("\n" + errorStyle.Render("Error: "+m.DriveDownloadError.Error()) + "\n")
		b.WriteString("\n" + FooterStyle.Render(" Esc back "))
	}
	return b.String()
}

func (m TUIModel) updateDriveDownload(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.ViewState = ViewDriveFileSelect
		return m, nil
	}
	return m, nil
}

func (m TUIModel) driveFolderFromIndex() *drive.FolderItem {
	if m.DriveFolderIndex <= 0 {
		return nil
	}
	idx := m.DriveFolderIndex - 1
	if idx < 0 || idx >= len(m.DriveFolders) {
		return nil
	}
	return &m.DriveFolders[idx]
}

func (m TUIModel) driveFolderSizeForCurrent() tea.Cmd {
	folder := m.driveFolderFromIndex()
	if folder == nil {
		return nil
	}
	return m.driveFolderSizeCmd(folder.ID)
}

func saveDriveConfig(m TUIModel) error {
	if m.DriveConfig == nil {
		return nil
	}
	return drive.SaveConfig(m.DriveConfig)
}
