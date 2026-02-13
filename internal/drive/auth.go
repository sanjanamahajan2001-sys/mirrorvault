package drive

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	deviceCodeURL = "https://oauth2.googleapis.com/device/code"
	tokenURL      = "https://oauth2.googleapis.com/token"
	userInfoURL   = "https://www.googleapis.com/oauth2/v2/userinfo"
)

type DeviceCode struct {
	DeviceCode      string
	UserCode        string
	VerificationURL string
	ExpiresIn       int
	Interval        int
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

type userInfoResponse struct {
	Email string `json:"email"`
}

type BrowserSession struct {
	AuthURL      string
	CodeCh       chan string
	ErrCh        chan error
	RedirectURL  string
	RedirectHost string
	ListenerHost string
	State        string
	Config       *oauth2.Config
	Shutdown     func()
}

func StartDeviceFlow(ctx context.Context, clientID string, scopes []string) (*DeviceCode, error) {
	if clientID == "" {
		return nil, fmt.Errorf("client id is required")
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("scopes are required")
	}

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scope", strings.Join(scopes, " "))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to build device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		bodyText := strings.TrimSpace(string(body))
		if bodyText != "" {
			return nil, fmt.Errorf("device code request failed: %s (%s)", resp.Status, bodyText)
		}
		return nil, fmt.Errorf("device code request failed: %s", resp.Status)
	}

	var payload struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURL string `json:"verification_url"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to parse device code response: %w", err)
	}
	if payload.Interval <= 0 {
		payload.Interval = 5
	}

	return &DeviceCode{
		DeviceCode:      payload.DeviceCode,
		UserCode:        payload.UserCode,
		VerificationURL: payload.VerificationURL,
		ExpiresIn:       payload.ExpiresIn,
		Interval:        payload.Interval,
	}, nil
}

func StartBrowserAuthSession(ctx context.Context, clientID, clientSecret string, scopes []string, redirectHost string, bindAll bool) (*BrowserSession, error) {
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("client id/secret are required")
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("scopes are required")
	}

	if redirectHost == "" {
		redirectHost = "localhost"
	}
	listenerHost := "127.0.0.1"
	if bindAll || (redirectHost != "localhost" && redirectHost != "127.0.0.1") {
		listenerHost = "0.0.0.0"
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:0", listenerHost))
	if err != nil {
		return nil, fmt.Errorf("failed to start local callback server: %w", err)
	}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	redirectURL := fmt.Sprintf("http://%s:%s/callback", redirectHost, port)
	state, err := randomState()
	if err != nil {
		listener.Close()
		return nil, err
	}

	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       scopes,
		RedirectURL:  redirectURL,
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("oauth state mismatch")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("State mismatch. You can close this window."))
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("missing authorization code")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Missing code. You can close this window."))
			return
		}
		codeCh <- code
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Authorization received. You can return to the terminal."))
	})

	server := &http.Server{Handler: mux}
	go func() {
		_ = server.Serve(listener)
	}()
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()

	authURL := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	return &BrowserSession{
		AuthURL:      authURL,
		CodeCh:       codeCh,
		ErrCh:        errCh,
		RedirectURL:  redirectURL,
		RedirectHost: redirectHost,
		ListenerHost: listenerHost,
		State:        state,
		Config:       conf,
		Shutdown:     func() { _ = server.Close() },
	}, nil
}

func ExchangeBrowserCode(ctx context.Context, session *BrowserSession, code string) (*oauth2.Token, error) {
	if session == nil || session.Config == nil {
		return nil, fmt.Errorf("browser session not initialized")
	}
	return session.Config.Exchange(ctx, code)
}

func PollForToken(ctx context.Context, clientID, clientSecret string, device *DeviceCode) (*tokenResponse, error) {
	if device == nil {
		return nil, fmt.Errorf("device code is required")
	}
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("client id/secret are required")
	}

	expiry := time.Now().Add(time.Duration(device.ExpiresIn) * time.Second)
	interval := time.Duration(device.Interval) * time.Second

	for {
		if time.Now().After(expiry) {
			return nil, fmt.Errorf("device code expired, please retry")
		}

		form := url.Values{}
		form.Set("client_id", clientID)
		form.Set("client_secret", clientSecret)
		form.Set("device_code", device.DeviceCode)
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, fmt.Errorf("failed to build token request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("token request failed: %w", err)
		}
		var payload tokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to parse token response: %w", err)
		}
		resp.Body.Close()

		if payload.Error == "" {
			return &payload, nil
		}
		switch payload.Error {
		case "authorization_pending":
			// keep polling
		case "slow_down":
			interval += 5 * time.Second
		default:
			if payload.ErrorDesc != "" {
				return nil, fmt.Errorf("token request failed: %s", payload.ErrorDesc)
			}
			return nil, fmt.Errorf("token request failed: %s", payload.Error)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func randomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to create oauth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func FetchAccountEmail(ctx context.Context, accessToken string) (string, error) {
	if accessToken == "" {
		return "", fmt.Errorf("access token is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("userinfo request failed: %s", resp.Status)
	}

	var payload userInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("failed to parse userinfo response: %w", err)
	}
	return payload.Email, nil
}
