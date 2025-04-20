package privatekey

import (
	"context"
	"crypto/rsa"
	"log"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	expireDuration  = time.Minute
	refreshInterval = 10 * time.Second
)

// Generator jwt generator
type Generator struct {
	mu             *sync.RWMutex
	privateKeyFile string
	privateKeyUser string
	privateKey     *rsa.PrivateKey
	token          string
	expiration     time.Time
}

// NewGenerator new renewer
func NewGenerator(privateKeyFile, privateKeyUser string) (*Generator, error) {
	signBytes, err := os.ReadFile(privateKeyFile)
	if err != nil {
		return nil, err
	}
	signKey, err := jwt.ParseRSAPrivateKeyFromPEM(signBytes)
	if err != nil {
		return nil, err
	}

	return &Generator{
		privateKeyFile: privateKeyFile,
		privateKeyUser: privateKeyUser,
		privateKey:     signKey,
		mu:             new(sync.RWMutex),
	}, nil
}

// Enabled privateKeyFile is exists
func (g *Generator) Enabled() bool {
	return g.privateKeyFile != ""
}

// Gen generate jwt token
func (g *Generator) Gen(ctx context.Context) (string, error) {
	iat := time.Now()
	exp := iat.Add(expireDuration)

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		Subject:   g.privateKeyUser,
		Issuer:    "wsgatet-client",
		IssuedAt:  jwt.NewNumericDate(iat),
		ExpiresAt: jwt.NewNumericDate(exp),
	})
	tokenString, err := token.SignedString(g.privateKey)

	if err != nil {
		return "", err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.token = tokenString
	g.expiration = exp
	return tokenString, nil
}

// Get access token
func (g *Generator) Get(ctx context.Context) (string, error) {
	g.mu.Lock()
	token := g.token
	expire := g.expiration
	g.mu.Unlock()
	// return cahced token if the token is valid
	if token != "" && time.Now().Before(expire) {
		return token, nil
	}
	token, err := g.Gen(ctx)
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
			_, err := g.Gen(ctx)
			if err != nil {
				log.Printf("Regularly generate token failed:%v", err)
			}
		}
	}
}
