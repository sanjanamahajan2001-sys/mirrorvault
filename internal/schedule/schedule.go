package schedule

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	SchedulesDir     = "/var/lib/mirrorvault"
	SchedulesFile    = "/var/lib/mirrorvault/schedules.json"
	SystemdUnitDir   = "/etc/systemd/system"
	CleanupTimerName = "mirrorvault-cleanup.timer"
	CleanupServiceName = "mirrorvault-cleanup.service"
)

type Schedule struct {
	Engine    string   `json:"engine"`
	Databases []string `json:"databases"`
	Time      string   `json:"time"` // Format: HH:MM
	TimerName string   `json:"timer_name"`
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
	for _, schedule := range schedules {
		if schedule.Engine == engine {
			for _, db := range databases {
				for _, scheduledDB := range schedule.Databases {
					if db == scheduledDB {
						duplicates = append(duplicates, db)
					}
				}
			}
		}
	}

	return duplicates, nil
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
	timerContent := fmt.Sprintf(`[Unit]
Description=MirrorVault daily backup for %s databases: %s
Requires=%s

[Timer]
%s
Persistent=true

[Install]
WantedBy=timers.target
`, schedule.Engine, dbList, CleanupServiceName, onCalendar)

	// Create service unit content
	// Set MIRRORVAULT_SCHEDULED=true to use daily-backups directory
	// Note: Since we're already running as root (User=root), we don't need sudo
	// and environment variables will be preserved
	envVars := fmt.Sprintf(`Environment="MIRRORVAULT_SCHEDULED=true"
Environment="MIRRORVAULT_SCHEDULED_ENGINE=%s"
Environment="MIRRORVAULT_SCHEDULED_DBS=%s"`, schedule.Engine, dbList)
	
	// Add password environment variable if provided
	// Format: MIRRORVAULT_<ENGINE>_PASSWORD
	if password != "" {
		envVarName := fmt.Sprintf("MIRRORVAULT_%s_PASSWORD", strings.ToUpper(schedule.Engine))
		envVars += fmt.Sprintf("\nEnvironment=\"%s=%s\"", envVarName, password)
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
func AddSchedule(engine string, databases []string, time string, password string) error {
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
		Engine:    engine,
		Databases: databases,
		Time:      time,
		TimerName: timerName,
	}

	// Load existing schedules
	schedules, err := LoadSchedules()
	if err != nil {
		return err
	}

	// Check if cleanup service exists, create if not
	// This ensures cleanup is set up on first schedule
	if len(schedules) == 0 {
		if err := CreateCleanupService(); err != nil {
			// Log error but continue - cleanup service creation failure shouldn't block scheduling
			// In production, you might want to handle this differently
		}
	}

	// Add new schedule
	schedules = append(schedules, schedule)

	// Save schedules
	if err := SaveSchedules(schedules); err != nil {
		return err
	}

	// Create systemd timer with password if provided
	if err := CreateSystemdTimer(schedule, password); err != nil {
		// If timer creation fails, remove the schedule from the list
		// Find and remove the schedule we just added
		for i, s := range schedules {
			if s.TimerName == timerName {
				schedules = append(schedules[:i], schedules[i+1:]...)
				SaveSchedules(schedules) // Try to save, ignore error
				break
			}
		}
		return err
	}

	return nil
}

// GetAllSchedules returns all schedules
func GetAllSchedules() ([]Schedule, error) {
	return LoadSchedules()
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
			
			// Stop and disable old timer
			exec.Command("sudo", "systemctl", "stop", timerName).Run()
			exec.Command("sudo", "systemctl", "disable", timerName).Run()
			
			// Remove old timer and service files
			timerPath := filepath.Join(SystemdUnitDir, timerName)
			serviceName := strings.Replace(timerName, ".timer", ".service", 1)
			servicePath := filepath.Join(SystemdUnitDir, serviceName)
			os.Remove(timerPath)
			os.Remove(servicePath)
			
			// Update time and regenerate timer name
			schedules[i].Time = newTime
			schedules[i].TimerName = GenerateTimerName(schedules[i].Engine, schedules[i].Databases, newTime)
			
			// Create new systemd timer with updated time
			// Preserve password from existing service file if it exists
			existingPassword := ""
			if servicePath != "" {
				// Try to read password from existing service file
				existingServiceContent, err := os.ReadFile(servicePath)
				if err == nil {
					// Extract password from environment variable
					lines := strings.Split(string(existingServiceContent), "\n")
					envVarPrefix := fmt.Sprintf("MIRRORVAULT_%s_PASSWORD=", strings.ToUpper(schedules[i].Engine))
					for _, line := range lines {
						if strings.Contains(line, envVarPrefix) {
							// Extract password value
							parts := strings.SplitN(line, "=", 2)
							if len(parts) == 2 {
								// Remove quotes if present
								existingPassword = strings.Trim(parts[1], `"`)
							}
							break
						}
					}
				}
			}
			
			if err := CreateSystemdTimer(schedules[i], existingPassword); err != nil {
				return fmt.Errorf("failed to create updated timer: %w", err)
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
