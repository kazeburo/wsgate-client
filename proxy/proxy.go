package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kazeburo/wsgate-client/token"
	"golang.org/x/net/websocket"
)

const (
	bufferSize = 0xFFFF
)

// Proxy proxy struct
type Proxy struct {
	server          *net.TCPListener
	listen          string
	timeout         time.Duration
	shutdownTimeout time.Duration
	upstream        string
	header          http.Header
	gr              token.Generator
	done            chan struct{}
}

// NewProxy create new proxy
func NewProxy(listen string, timeout, shutdownTimeout time.Duration, upstream string, header http.Header, gr token.Generator) (*Proxy, error) {
	addr, err := net.ResolveTCPAddr("tcp", listen)
	if err != nil {
		return nil, err
	}
	server, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Proxy{
		server:          server,
		listen:          listen,
		timeout:         timeout,
		shutdownTimeout: shutdownTimeout,
		upstream:        upstream,
		header:          header,
		gr:              gr,
		done:            make(chan struct{}),
	}, nil
}

// Start start new proxy
func (p *Proxy) Start(ctx context.Context) error {
	wg := &sync.WaitGroup{}
	defer func() {
		c := make(chan struct{})
		go func() {
			defer close(c)
			wg.Wait()
		}()
		select {
		case <-c:
			return
		case <-time.After(p.shutdownTimeout):
			return
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			p.server.Close()
		}
	}()
	for {
		conn, err := p.server.AcceptTCP()
		if err != nil {
			if ne, ok := err.(net.Error); ok {
				if ne.Temporary() {
					continue
				}
			}
			if strings.Contains(err.Error(), "use of closed network connection") {
				select {
				case <-ctx.Done():
					return nil
				default:
					// fallthrough
				}
			}
			return err
		}

		conn.SetNoDelay(true)
		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			p.handleConn(ctx, c)
		}(conn)
	}
}

var wsRegexp = regexp.MustCompile("^http")

func (p *Proxy) connectWS(ctx context.Context) (net.Conn, error) {
	wsURL := wsRegexp.ReplaceAllString(p.upstream, "ws")
	// log.Printf("connecting to %s", wsURL)
	wsConf, err := websocket.NewConfig(wsURL, p.upstream)
	if err != nil {
		return nil, fmt.Errorf("NewConfig failed: %v", err)
	}

	h2 := make(http.Header, len(p.header))
	for k, vv := range p.header {
		vv2 := make([]string, len(vv))
		copy(vv2, vv)
		h2[k] = vv2
	}
	wsConf.Header = h2
	if p.gr.Enabled() {
		t, tErr := p.gr.Get(ctx)
		if tErr != nil {
			return nil, fmt.Errorf("Failed to generate token: %v", tErr)
		}
		wsConf.Header.Add("Authorization", fmt.Sprintf("Bearer %s", t))
	}

	wsConf.Dialer = &net.Dialer{
		Timeout:   p.timeout,
		KeepAlive: 10 * time.Second,
	}
	conn, err := websocket.DialConfig(wsConf)
	if err != nil {
		return nil, fmt.Errorf("Dial to %s fail: %v", p.upstream, err)
	}
	conn.PayloadType = websocket.BinaryFrame
	return conn, err
}

func (p *Proxy) handleConn(ctx context.Context, c net.Conn) error {
	s, err := p.connectWS(ctx)
	if err != nil {
		log.Printf("Failed to connect backend:%v listen:%s client:%s", err, p.listen, c.RemoteAddr().String())
		c.Close()
		return err
	}

	doneCh := make(chan bool)
	goClose := false

	// client => upstream
	go func() {
		defer func() { doneCh <- true }()
		_, err := io.Copy(s, c)
		if err != nil {
			if !goClose {
				log.Printf("Copy from client: %v", err)
				return
			}
		}
		return
	}()

	// upstream => client
	go func() {
		defer func() { doneCh <- true }()
		_, err := io.Copy(c, s)
		if err != nil {
			if !goClose {
				log.Printf("Copy from upstream: %v", err)
				return
			}
		}
		return
	}()

	<-doneCh
	goClose = true
	s.Close()
	c.Close()
	<-doneCh
	return nil
}
