package privatekey

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func generateTestKeys() ([]byte, *rsa.PublicKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return privateKeyPEM, &privateKey.PublicKey, nil
}

func TestGet(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	privateKeyPEM, publicKey, err := generateTestKeys()
	assert.NoError(err)

	// Create a temporary private key file
	tmpFile, err := os.CreateTemp("", "privatekey-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()
	// Write a dummy private key to the file
	_, err = tmpFile.Write(privateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	// Create a new Generator
	gen, err := NewGenerator(tmpFile.Name(), "test-user")
	assert.NoError(err)

	// Call the Get method
	token, err := gen.Get(ctx)
	assert.NoError(err)

	// Check if the token is not empty
	assert.NotEmpty(token)
	// Check if the token is a valid JWT
	claims := &jwt.RegisteredClaims{}
	_, err = jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
		return publicKey, nil
	}, jwt.WithValidMethods([]string{"RS256", "RS384", "RS512"}))
	assert.NoError(err)
	assert.Equal(claims.Subject, "test-user")
}
