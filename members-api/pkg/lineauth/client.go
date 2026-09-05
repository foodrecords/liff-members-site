package lineauth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Profile struct {
	UserID        string `json:"userId"`
	DisplayName   string `json:"displayName"`
	PictureURL    string `json:"pictureUrl"`
	StatusMessage string `json:"statusMessage"`
}

func baseURL() string {
	if value := strings.TrimRight(os.Getenv("LINE_API_BASE_URL"), "/"); value != "" {
		return value
	}
	return "https://api.line.me"
}

func VerifyAccessToken(token string) error {
	expected := allowedChannelIDs()
	if len(expected) == 0 {
		return errors.New("LINE login channel IDs are not configured")
	}
	endpoint := baseURL() + "/oauth2/v2.1/verify?access_token=" + url.QueryEscape(token)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Get(endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("invalid LINE access token")
	}
	var body struct {
		ClientID  string `json:"client_id"`
		ExpiresIn int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return err
	}
	if !expected[body.ClientID] || body.ExpiresIn <= 0 {
		return errors.New("LINE access token channel mismatch or expired")
	}
	return nil
}

func allowedChannelIDs() map[string]bool {
	configured := os.Getenv("LINE_LOGIN_CHANNEL_IDS")
	if strings.TrimSpace(configured) == "" {
		configured = os.Getenv("LINE_LOGIN_CHANNEL_ID")
	}
	result := make(map[string]bool)
	for _, value := range strings.Split(configured, ",") {
		if value = strings.TrimSpace(value); value != "" {
			result[value] = true
		}
	}
	return result
}

func GetProfile(token string) (*Profile, error) {
	if err := VerifyAccessToken(token); err != nil {
		return nil, err
	}
	req, _ := http.NewRequest(http.MethodGet, baseURL()+"/v2/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("invalid LINE token")
	}
	var profile Profile
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&profile); err != nil {
		return nil, err
	}
	if profile.UserID == "" {
		return nil, errors.New("missing LINE user")
	}
	return &profile, nil
}
