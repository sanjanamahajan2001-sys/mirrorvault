package credentials

import (
	"errors"
	"fmt"
	"strings"
	"syscall"

	"golang.org/x/term"
)

var ErrAuthFailed = errors.New("authentication failed")

const maxAttempts = 2

// Prompt asks for a password for a given engine.
// It hides input and limits attempts.
func Prompt(engine string) (string, error) {
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		fmt.Printf("\nEnter password for %s (attempt %d/%d): ",
			engine, attempt, maxAttempts)

		bytePwd, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return "", err
		}

		password := strings.TrimSpace(string(bytePwd))

		if password == "" {
			fmt.Println("Password cannot be empty.")
			continue
		}

		// We don't validate yet (that happens in Phase 6)
		return password, nil
	}

	return "", ErrAuthFailed
}
