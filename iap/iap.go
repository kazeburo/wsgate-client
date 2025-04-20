package iap

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2/google"
)

const (
	expireDuration  = time.Hour
	refreshInterval = 600 * time.Second
)

// Generator jwt generator
type Generator struct {
	mu             *sync.RWMutex
	credentialFile string
	credentialJSON *[]byte
	iapClientID    string
	token          string
	expiration     time.Time
	tokenEndpoint  string
	signJWT        func([]byte) (string, error)
}

// NewGenerator new renewer
func NewGenerator(credentialFile string, iapClientID string) (*Generator, error) {
	sa, err := os.ReadFile(credentialFile)
	if err != nil {
		return nil, err
	}

	// Check if the file is a valid JSON
	_, err = google.JWTConfigFromJSON(sa)
	if err != nil {
		return nil, err
	}

	return &Generator{
		credentialFile: credentialFile,
		credentialJSON: &sa,
		iapClientID:    iapClientID,
		tokenEndpoint:  "https://oauth2.googleapis.com/token",
		mu:             new(sync.RWMutex),
	}, nil
}

// Enabled privateKeyFile is exists
func (g *Generator) Enabled() bool {
	return *g.credentialJSON != nil
}

// GetToken with service account
func (g *Generator) GetToken(ctx context.Context) (string, error) {
	conf, err := google.JWTConfigFromJSON(*g.credentialJSON)
	if err != nil {
		return "", err
	}

	// IAPのAudience (OAuth2クライアントID)
	audience := fmt.Sprintf("%s.apps.googleusercontent.com", g.iapClientID)

	// JWTの作成
	iat := time.Now()
	claims := jwtlib.MapClaims{
		"iss":             conf.Email,
		"sub":             conf.Email,
		"aud":             "https://oauth2.googleapis.com/token",
		"iat":             iat.Unix(),
		"exp":             iat.Add(expireDuration).Unix(),
		"target_audience": audience,
	}

	token := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims)
	privateKey := conf.PrivateKey
	var signedJWT string
	if g.signJWT != nil {
		signedJWT, err = g.signJWT(privateKey)
		if err != nil {
			return "", err
		}
	} else {
		signedJWT, err = token.SignedString(privateKey)
		if err != nil {
			return "", err
		}
	}

	// Google OAuth2 エンドポイントにJWTを使ってトークンを取得
	resp, err := http.PostForm(g.tokenEndpoint, map[string][]string{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {signedJWT},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get token: %s", resp.Status)
	}

	// JSONからaccess_tokenを抽出
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.token = tokenResp.AccessToken
	g.expiration = iat.Add(expireDuration)
	return tokenResp.AccessToken, nil
}

// Get access token
func (g *Generator) Get(ctx context.Context) (string, error) {
	g.mu.Lock()
	token := g.token
	expiration := g.expiration
	g.mu.Unlock()
	// return cahced token if the token is valid
	if token != "" && time.Now().Before(expiration) {
		return token, nil
	}
	token, err := g.GetToken(ctx)
	if err != nil {
		return token, err
	}
	return token, nil
}

// Run refresh token regularly
func (g *Generator) Run(ctx context.Context) {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := g.GetToken(ctx)
			if err != nil {
				log.Printf("Regularly renewToken failed:%v", err)
			}
		}
	}
}
