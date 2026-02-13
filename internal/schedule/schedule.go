package schedule

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mirrorvault/internal/backup/plan"
)

const (
	SchedulesDir       = "/var/lib/mirrorvault"
	SchedulesFile      = "/var/lib/mirrorvault/schedules.json"
	SystemdUnitDir     = "/etc/systemd/system"
	CleanupTimerName   = "mirrorvault-cleanup.timer"
	CleanupServiceName = "mirrorvault-cleanup.service"
)

type Schedule struct {
	Engine      string   `json:"engine"`
	Databases   []string `json:"databases"`
	Time        string   `json:"time"` // Format: HH:MM
	TimerName   string   `json:"timer_name"`
	Compression string   `json:"compression,omitempty"`
}

type ScheduleStore struct {
	Schedules []Schedule `json:"schedules"`
}

// LoadSchedules loads all schedules from the JSON file
func LoadSchedules() ([]Schedule, error) {
	data, err := os.ReadFile(SchedulesFile)
	if os.IsNotExist(err) {
		return []Schedule{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read schedules file: %w", err)
	}

	var store ScheduleStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("failed to parse schedules file: %w", err)
	}

	return store.Schedules, nil
}

// SaveSchedules saves all schedules to the JSON file
func SaveSchedules(schedules []Schedule) error {
	// Ensure directory exists
	if err := os.MkdirAll(SchedulesDir, 0755); err != nil {
		return fmt.Errorf("failed to create schedules directory: %w", err)
	}

	store := ScheduleStore{Schedules: schedules}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal schedules: %w", err)
	}

	if err := os.WriteFile(SchedulesFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write schedules file: %w", err)
	}

	return nil
}

// CheckDuplicate checks if any of the databases are already scheduled
func CheckDuplicate(engine string, databases []string) ([]string, error) {
	schedules, err := LoadSchedules()
	if err != nil {
		return nil, err
	}

	duplicates := []string{}
	seen := make(map[string]bool)
	for _, schedule := range schedules {
		if schedule.Engine == engine {
			if containsAllDatabases(schedule.Databases) {
				for _, db := range databases {
					if !seen[db] {
						seen[db] = true
						duplicates = append(duplicates, db)
					}
				}
				continue
			}
			if containsAllDatabases(databases) {
				for _, scheduledDB := range schedule.Databases {
					if !seen[scheduledDB] {
						seen[scheduledDB] = true
						duplicates = append(duplicates, scheduledDB)
					}
				}
				continue
			}
			for _, db := range databases {
				for _, scheduledDB := range schedule.Databases {
					if db == scheduledDB && !seen[db] {
						seen[db] = true
						duplicates = append(duplicates, db)
					}
				}
			}
		}
	}

	return duplicates, nil
}

func containsAllDatabases(databases []string) bool {
	for _, name := range databases {
		if name == plan.AllDatabasesName {
			return true
		}
	}
	return false
}

// GenerateTimerName generates a unique timer name for a schedule
func GenerateTimerName(engine string, databases []string, time string) string {
	// Create a hash-like identifier from engine, databases, and time
	dbList := strings.Join(databases, "-")
	timeClean := strings.ReplaceAll(time, ":", "-")
	identifier := fmt.Sprintf("%s-%s-%s", strings.ToLower(engine), dbList, timeClean)
	// Limit length and sanitize
	identifier = strings.ReplaceAll(identifier, "/", "-")
	if len(identifier) > 50 {
		identifier = identifier[:50]
	}
	return fmt.Sprintf("mirrorvault-%s.timer", identifier)
}

// findMirrorVaultPath finds the path to the mirrorvault binary
func findMirrorVaultPath() (string, error) {
	// First, try to get the path of the currently running executable
	// This works if the user ran mirrorvault from a specific path
	if execPath, err := os.Executable(); err == nil {
		// Resolve symlinks to get the actual path
		if resolvedPath, err := filepath.EvalSymlinks(execPath); err == nil {
			if _, err := os.Stat(resolvedPath); err == nil {
				return resolvedPath, nil
			}
		}
		// If symlink resolution failed, try the original path
		if _, err := os.Stat(execPath); err == nil {
			return execPath, nil
		}
	}

	// Try common locations
	commonPaths := []string{
		"/usr/local/bin/mirrorvault",
		"/usr/bin/mirrorvault",
		"/bin/mirrorvault",
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Try using 'which' command
	cmd := exec.Command("which", "mirrorvault")
	output, err := cmd.Output()
	if err == nil {
		path := strings.TrimSpace(string(output))
		if path != "" {
			return path, nil
		}
	}

	// Try using 'whereis' command
	cmd = exec.Command("whereis", "-b", "mirrorvault")
	output, err = cmd.Output()
	if err == nil {
		parts := strings.Fields(string(output))
		if len(parts) > 1 {
			return parts[1], nil
		}
	}

	return "", fmt.Errorf("mirrorvault binary not found. Please install it to /usr/local/bin/ or ensure it's in PATH")
}

// CreateSystemdTimer creates a systemd timer unit for the schedule
// password is optional and only needed if the engine requires authentication
func CreateSystemdTimer(schedule Schedule, password string) error {
	// Find mirrorvault binary path
	mirrorvaultPath, err := findMirrorVaultPath()
	if err != nil {
		return fmt.Errorf("failed to find mirrorvault binary: %w", err)
	}

	// Parse time (HH:MM format)
	timeParts := strings.Split(schedule.Time, ":")
	if len(timeParts) != 2 {
		return fmt.Errorf("invalid time format: %s (expected HH:MM)", schedule.Time)
	}

	hour := timeParts[0]
	minute := timeParts[1]

	// Create OnCalendar expression (daily at specified time)
	onCalendar := fmt.Sprintf("OnCalendar=*-*-* %s:%s:00", hour, minute)

	// Build databases list for the command
	dbList := strings.Join(schedule.Databases, " ")

	// Create timer unit content
	// Note: Timer units automatically trigger their corresponding .service unit
	// No need to explicitly reference the service or cleanup service
	timerContent := fmt.Sprintf(`[Unit]
Description=MirrorVault daily backup for %s databases: %s

[Timer]
%s
Persistent=true

[Install]
WantedBy=timers.target
`, schedule.Engine, dbList, onCalendar)

	// Create service unit content
	// Set MIRRORVAULT_SCHEDULED=true to use daily-backups directory
	// Note: Since we're already running as root (User=root), we don't need sudo
	// and environment variables will be preserved
	envVars := fmt.Sprintf(`Environment="MIRRORVAULT_SCHEDULED=true"
Environment="MIRRORVAULT_SCHEDULED_ENGINE=%s"
Environment="MIRRORVAULT_SCHEDULED_DBS=%s"`, schedule.Engine, dbList)
	if schedule.Compression != "" {
		envVars += fmt.Sprintf("\nEnvironment=\"MV_BACKUP_COMPRESSION=%s\"", schedule.Compression)
	}

	// Add password environment variable if provided (fallback)
	// Format: MIRRORVAULT_<ENGINE>_PASSWORD
	if password != "" {
		envVarName := fmt.Sprintf("MIRRORVAULT_%s_PASSWORD", strings.ToUpper(schedule.Engine))
		envVars += fmt.Sprintf("\nEnvironment=\"%s=%s\"", envVarName, password)
	}

	// Use EnvironmentFile if a secret exists (preferred)
	if secretPath, ok := secretFileExists(schedule.TimerName); ok {
		envVars += fmt.Sprintf("\nEnvironmentFile=%s", secretPath)
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=MirrorVault daily backup for %s databases: %s
After=network.target

[Service]
Type=oneshot
ExecStart=%s backup
%s
StandardOutput=journal
StandardError=journal
User=root
`, schedule.Engine, dbList, mirrorvaultPath, envVars)

	timerPath := filepath.Join(SystemdUnitDir, schedule.TimerName)
	serviceName := strings.Replace(schedule.TimerName, ".timer", ".service", 1)
	servicePath := filepath.Join(SystemdUnitDir, serviceName)

	// Write timer unit
	if err := os.WriteFile(timerPath, []byte(timerContent), 0644); err != nil {
		return fmt.Errorf("failed to write timer unit: %w", err)
	}

	// Write service unit
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service unit: %w", err)
	}

	// Reload systemd and enable timer
	cmd := exec.Command("sudo", "systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	cmd = exec.Command("sudo", "systemctl", "enable", schedule.TimerName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable timer: %w", err)
	}

	cmd = exec.Command("sudo", "systemctl", "start", schedule.TimerName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start timer: %w", err)
	}

	return nil
}

// AddSchedule adds a new schedule and creates the systemd timer
// password is optional and only needed if the engine requires authentication
func AddSchedule(engine string, databases []string, time string, compression string, password string) error {
	// Check for duplicates
	duplicates, err := CheckDuplicate(engine, databases)
	if err != nil {
		return err
	}
	if len(duplicates) > 0 {
		return fmt.Errorf("backup already scheduled for databases: %s", strings.Join(duplicates, ", "))
	}

	// Generate timer name
	timerName := GenerateTimerName(engine, databases, time)

	// Create schedule
	schedule := Schedule{
		Engine:      engine,
		Databases:   databases,
		Time:        time,
		TimerName:   timerName,
		Compression: compression,
	}

	// Load existing schedules
	schedules, err := LoadSchedules()
	if err != nil {
		return err
	}

	// Check if cleanup service exists, create if not
	// This ensures cleanup is set up on first schedule
	if len(schedules) == 0 {
		if err := ensureCleanupSchedule(); err != nil {
			// Log error but continue - cleanup setup failure shouldn't block scheduling
		}
	}

	// Add new schedule
	schedules = append(schedules, schedule)

	// Save schedules
	if err := SaveSchedules(schedules); err != nil {
		return err
	}

	// Store credentials securely (if provided)
	if _, err := writeSecretFile(timerName, engine, password); err != nil {
		return err
	}

	// Create backend schedule entry
	if err := applyScheduleBackend(schedule); err != nil {
		// If timer creation fails, remove the schedule from the list
		// Find and remove the schedule we just added
		for i, s := range schedules {
			if s.TimerName == timerName {
				schedules = append(schedules[:i], schedules[i+1:]...)
				SaveSchedules(schedules) // Try to save, ignore error
				break
			}
		}
		_ = removeSecretFile(timerName)
		return err
	}

	return nil
}

// GetAllSchedules returns all schedules
func GetAllSchedules() ([]Schedule, error) {
	return LoadSchedules()
}

// FixExistingTimers removes the incorrect Requires dependency from all existing timer units
// This fixes timers that were created before the bug fix
func FixExistingTimers() error {
	if detectBackend() != backendSystemd {
		return nil
	}
	// Find all mirrorvault timer files (excluding cleanup timer)
	timerFiles, err := filepath.Glob(filepath.Join(SystemdUnitDir, "mirrorvault-*.timer"))
	if err != nil {
		return fmt.Errorf("failed to find timer files: %w", err)
	}

	fixedCount := 0
	for _, timerPath := range timerFiles {
		// Skip cleanup timer - it correctly requires its service
		if strings.Contains(timerPath, "mirrorvault-cleanup.timer") {
			continue
		}

		// Read timer file
		content, err := os.ReadFile(timerPath)
		if err != nil {
			continue // Skip if can't read
		}

		// Check if it has the incorrect Requires line
		contentStr := string(content)
		if !strings.Contains(contentStr, "Requires=mirrorvault-cleanup.service") {
			continue // Already fixed or doesn't have the issue
		}

		// Remove the Requires line
		lines := strings.Split(contentStr, "\n")
		newLines := []string{}
		for _, line := range lines {
			if strings.TrimSpace(line) != "Requires=mirrorvault-cleanup.service" {
				newLines = append(newLines, line)
			}
		}

		// Write back the fixed content
		newContent := strings.Join(newLines, "\n")
		if err := os.WriteFile(timerPath, []byte(newContent), 0644); err != nil {
			continue // Skip if can't write
		}

		fixedCount++

		// Restart the timer
		timerName := filepath.Base(timerPath)
		exec.Command("sudo", "systemctl", "daemon-reload").Run()
		exec.Command("sudo", "systemctl", "restart", timerName).Run()
	}

	if fixedCount > 0 {
		// Reload systemd once after all fixes
		cmd := exec.Command("sudo", "systemctl", "daemon-reload")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to reload systemd: %w", err)
		}
	}

	return nil
}

// RemoveSchedule removes a schedule by timer name and stops/removes the systemd timer
func RemoveSchedule(timerName string) error {
	// Load existing schedules
	schedules, err := LoadSchedules()
	if err != nil {
		return err
	}

	// Find and remove the schedule
	found := false
	newSchedules := []Schedule{}
	for _, s := range schedules {
		if s.TimerName == timerName {
			found = true
			// Stop and disable the timer
			exec.Command("sudo", "systemctl", "stop", timerName).Run()
			exec.Command("sudo", "systemctl", "disable", timerName).Run()

			// Remove timer and service files
			timerPath := filepath.Join(SystemdUnitDir, timerName)
			serviceName := strings.Replace(timerName, ".timer", ".service", 1)
			servicePath := filepath.Join(SystemdUnitDir, serviceName)

			os.Remove(timerPath)
			os.Remove(servicePath)

			// Reload systemd
			exec.Command("sudo", "systemctl", "daemon-reload").Run()
			if err := removeScheduleBackend(timerName); err != nil {
				return err
			}
			_ = removeSecretFile(timerName)
		} else {
			newSchedules = append(newSchedules, s)
		}
	}

	if !found {
		return fmt.Errorf("schedule with timer name %s not found", timerName)
	}

	// Save updated schedules
	return SaveSchedules(newSchedules)
}

// RemoveAllSchedules removes all schedules
func RemoveAllSchedules() error {
	schedules, err := LoadSchedules()
	if err != nil {
		return err
	}

	// Remove each schedule
	for _, s := range schedules {
		if err := RemoveSchedule(s.TimerName); err != nil {
			// Log error but continue
			fmt.Printf("Warning: failed to remove schedule %s: %v\n", s.TimerName, err)
		}
	}

	// Clear the schedules file
	return SaveSchedules([]Schedule{})
}

// UpdateScheduleTime updates the time for an existing schedule
func UpdateScheduleTime(timerName string, newTime string) error {
	// Load existing schedules
	schedules, err := LoadSchedules()
	if err != nil {
		return err
	}

	// Find the schedule
	found := false
	for i := range schedules {
		if schedules[i].TimerName == timerName {
			found = true

			// Remove old backend entry
			if err := removeScheduleBackend(timerName); err != nil {
				return err
			}

			// Update time and regenerate timer name
			schedules[i].Time = newTime
			schedules[i].TimerName = GenerateTimerName(schedules[i].Engine, schedules[i].Databases, newTime)

			// Preserve credentials file (if any)
			if err := renameSecretFile(timerName, schedules[i].TimerName); err != nil {
				return fmt.Errorf("failed to update credentials file: %w", err)
			}

			if err := applyScheduleBackend(schedules[i]); err != nil {
				return fmt.Errorf("failed to create updated schedule: %w", err)
			}

			break
		}
	}

	if !found {
		return fmt.Errorf("schedule with timer name %s not found", timerName)
	}

	// Save updated schedules
	return SaveSchedules(schedules)
}

func applyScheduleBackend(schedule Schedule) error {
	switch detectBackend() {
	case backendSystemd:
		return CreateSystemdTimer(schedule, "")
	case backendCron:
		mirrorvaultPath, err := findMirrorVaultPath()
		if err != nil {
			return fmt.Errorf("failed to find mirrorvault binary: %w", err)
		}
		return createCronSchedule(schedule, mirrorvaultPath)
	default:
		return fmt.Errorf("no supported scheduler backend found (systemd or cron required)")
	}
}

func removeScheduleBackend(timerName string) error {
	switch detectBackend() {
	case backendSystemd:
		// Stop and disable the timer
		exec.Command("sudo", "systemctl", "stop", timerName).Run()
		exec.Command("sudo", "systemctl", "disable", timerName).Run()

		// Remove timer and service files
		timerPath := filepath.Join(SystemdUnitDir, timerName)
		serviceName := strings.Replace(timerName, ".timer", ".service", 1)
		servicePath := filepath.Join(SystemdUnitDir, serviceName)
		os.Remove(timerPath)
		os.Remove(servicePath)

		// Reload systemd
		exec.Command("sudo", "systemctl", "daemon-reload").Run()
		return nil
	case backendCron:
		return removeCronSchedule(timerName)
	default:
		return fmt.Errorf("no supported scheduler backend found (systemd or cron required)")
	}
}

func ensureCleanupSchedule() error {
	switch detectBackend() {
	case backendSystemd:
		return CreateCleanupService()
	case backendCron:
		return CreateCleanupCron()
	default:
		return fmt.Errorf("no supported scheduler backend found (systemd or cron required)")
	}
}
