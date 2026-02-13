package drive

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type Client struct {
	Service *drive.Service
}

func NewClient(ctx context.Context, cfg *Config) (*Client, error) {
	if cfg == nil || cfg.RefreshToken == "" {
		return nil, fmt.Errorf("drive config not connected")
	}
	clientID := os.Getenv("MV_GDRIVE_CLIENT_ID")
	clientSecret := os.Getenv("MV_GDRIVE_CLIENT_SECRET")
	if clientID == "" && cfg.ClientID != "" {
		clientID = cfg.ClientID
	}
	if clientSecret == "" && cfg.ClientSecret != "" {
		clientSecret = cfg.ClientSecret
	}
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("missing MV_GDRIVE_CLIENT_ID or MV_GDRIVE_CLIENT_SECRET")
	}
	tokenURI := cfg.TokenURI
	if tokenURI == "" {
		tokenURI = tokenURL
	}

	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{scopeFromConfig(cfg)},
	}

	token := &oauth2.Token{
		RefreshToken: cfg.RefreshToken,
		TokenType:    "Bearer",
	}
	ts := conf.TokenSource(ctx, token)

	service, err := drive.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to create drive service: %w", err)
	}

	return &Client{Service: service}, nil
}

func scopeFromConfig(cfg *Config) string {
	if cfg != nil && cfg.Scope != "" {
		return cfg.Scope
	}
	return "https://www.googleapis.com/auth/drive.file"
}
