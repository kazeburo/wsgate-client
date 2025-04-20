package iap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createFakeTokenServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Errorf("unexpected grant_type: %s", r.Form.Get("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"access_token": "fake-access-token",
		})
	}))
}

func TestGetTokenReturnsToken(t *testing.T) {
	assert := assert.New(t)
	server := createFakeTokenServer(t)
	defer server.Close()

	// 偽のサービスアカウントJSONを用意
	sa := `{
		"type": "service_account",
		"client_email": "test@test.iam.gserviceaccount.com",
		"private_key": "-----BEGIN PRIVATE KEY-----\nFAKEKEY\n-----END PRIVATE KEY-----\n"
	}`
	tmpFile := "test-sa.json"
	if err := os.WriteFile(tmpFile, []byte(sa), 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile)

	// 実行
	gen, err := NewGenerator(tmpFile, "test-iap-client-id")
	assert.NoError(err)

	// 一時的にエンドポイントを差し替える（対象URLハードコード対応のため）
	originalEndpoint := gen.tokenEndpoint
	gen.tokenEndpoint = server.URL
	gen.signJWT = func(s []byte) (string, error) {
		assert.Contains(string(s), "FAKEKEY")
		return "fake-signed-jwt", nil
	}
	defer func() {
		gen.tokenEndpoint = originalEndpoint
		gen.signJWT = nil
	}()

	token, err := gen.GetToken(context.Background())
	assert.NoError(err)
	assert.Equal(token, "fake-access-token")

	assert.Equal(gen.token, "fake-access-token")
	assert.NotEqual(gen.expiration, 0)
}
