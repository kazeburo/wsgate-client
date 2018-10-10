package iap

// based on https://github.com/b4b4r07/iap_curl/blob/master/iap.go

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jws"
	"golang.org/x/oauth2/jwt"
)

const (
	tokenURI        = "https://www.googleapis.com/oauth2/v4/token"
	expireSec       = 3600
	refreshInterval = 600
)

// Generator jwt generator
type Generator struct {
	mu             *sync.RWMutex
	jwtConfig      *jwt.Config
	credentialFile string
	iapClientID    string
	signKey        *rsa.PrivateKey
	tkn            string
	exp            time.Time
	cli            *http.Client
}

// NewGenerator new renewer
func NewGenerator(credentialFile string, iapClientID string) (*Generator, error) {
	sa, err := ioutil.ReadFile(credentialFile)
	if err != nil {
		return nil, err
	}
	conf, err := google.JWTConfigFromJSON(sa)
	if err != nil {
		return nil, err
	}
	signKey, err := readRsaPrivateKey(conf.PrivateKey)
	if err != nil {
		return nil, err
	}

	return &Generator{
		jwtConfig:      conf,
		credentialFile: credentialFile,
		iapClientID:    iapClientID,
		signKey:        signKey,
		mu:             new(sync.RWMutex),
	}, nil
}

// Enabled privateKeyFile is exists
func (g *Generator) Enabled() bool {
	return g.credentialFile != ""
}

func readRsaPrivateKey(bytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(bytes)
	if block == nil {
		return nil, errors.New("invalid private key data")
	}

	var key *rsa.PrivateKey
	var err error
	if block.Type == "RSA PRIVATE KEY" {
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
	} else if block.Type == "PRIVATE KEY" {
		keyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		var ok bool
		key, ok = keyInterface.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("not RSA private key")
		}
	} else {
		return nil, fmt.Errorf("invalid private key type: %s", block.Type)
	}

	key.Precompute()

	if err := key.Validate(); err != nil {
		return nil, err
	}

	return key, nil
}

// GetToken with service account
func (g *Generator) GetToken(ctx context.Context) (string, error) {
	iat := time.Now()
	exp := iat.Add(expireSec * time.Second)
	jwt := &jws.ClaimSet{
		Iss: g.jwtConfig.Email,
		Aud: tokenURI,
		Iat: iat.Unix(),
		Exp: exp.Unix(),
		PrivateClaims: map[string]interface{}{
			"target_audience": g.iapClientID,
		},
	}
	jwsHeader := &jws.Header{
		Algorithm: "RS256",
		Typ:       "JWT",
	}

	msg, err := jws.Encode(jwsHeader, jwt, g.signKey)
	if err != nil {
		return "", err
	}

	v := url.Values{}
	v.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	v.Set("assertion", msg)

	hc := oauth2.NewClient(ctx, nil)
	resp, err := hc.PostForm(tokenURI, v)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var tokenRes struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		IDToken     string `json:"id_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}

	if err := json.Unmarshal(body, &tokenRes); err != nil {
		return "", err
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.tkn = tokenRes.IDToken
	g.exp = exp
	return tokenRes.IDToken, nil
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
	token, err := g.GetToken(ctx)
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
			_, err := g.GetToken(ctx)
			if err != nil {
				log.Printf("Regularly renewToken failed:%v", err)
			}
		}
	}
}
