package privatekey

import (
	"context"
	"crypto/rsa"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
)

const (
	expireSec       = 60
	refreshInterval = 10
)

// Generator jwt generator
type Generator struct {
	mu             *sync.RWMutex
	privateKeyFile string
	privateKeyUser string
	signKey        *rsa.PrivateKey
	tkn            string
	exp            time.Time
	cli            *http.Client
}

// NewGenerator new renewer
func NewGenerator(privateKeyFile, privateKeyUser string) (*Generator, error) {
	signBytes, err := ioutil.ReadFile(privateKeyFile)
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
		signKey:        signKey,
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
	exp := iat.Add(expireSec * time.Second)
	t := jwt.NewWithClaims(jwt.GetSigningMethod("RS256"), jwt.StandardClaims{
		IssuedAt:  iat.Unix(),
		ExpiresAt: exp.Unix(),
		Issuer:    "wsgatet-client",
		Subject:   g.privateKeyUser,
	})
	tokenString, err := t.SignedString(g.signKey)
	if err != nil {
		return "", err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tkn = tokenString
	g.exp = exp
	return tokenString, nil
}

// Get access token
func (g *Generator) Get(ctx context.Context) (string, error) {
	g.mu.Lock()
	token := g.tkn
	expire := g.exp
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
	ticker := time.NewTicker(refreshInterval * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case _ = <-ticker.C:
			_, err := g.Gen(ctx)
			if err != nil {
				log.Printf("Regularly generate token failed:%v", err)
			}
		}
	}
}
