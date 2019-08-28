package proxy

import (
	"github.com/kazeburo/wsgate-client/defaults"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
	"time"
)

func TestNewProxy(t *testing.T) {
	assert := assert.New(t)

	listen := "127.0.0.1:14514"
	timeout, shutdownTimeout := time.Duration(10), time.Duration(10)
	upstream := "test"
	header := make(http.Header)
	header.Add("X-Test", "blah")
	gr := defaults.NewGenerator()

	proxy, err := NewProxy(listen, timeout, shutdownTimeout, upstream, header, gr)
	assert.Nil(err)
	if assert.NotNil(proxy) {
		assert.NotNil(proxy.server)
		assert.Equal(proxy.listen, listen)
		assert.Equal(proxy.timeout, timeout)
		assert.Equal(proxy.shutdownTimeout, shutdownTimeout)
		assert.Equal(proxy.upstream, upstream)
		assert.Equal(proxy.gr, gr)
		assert.NotNil(proxy.done)
	}

	_, err = NewProxy("invalidaddress", timeout, shutdownTimeout, upstream, header, gr)
	assert.NotNil(err)
}

func TestStart(t *testing.T) {
}
