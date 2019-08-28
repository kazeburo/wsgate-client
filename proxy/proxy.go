package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kazeburo/wsgate-client/token"
)

const (
	bufferSize = 0xFFFF
)

var pool = sync.Pool{
	New: func() interface{} { return make([]byte, 256*1024) },
}

// Proxy proxy struct
type Proxy struct {
	server            *net.TCPListener
	listen            string
	timeout           time.Duration
	shutdownTimeout   time.Duration
	upstream          string
	enableCompression bool
	header            http.Header
	gr                token.Generator
	done              chan struct{}
}

// NewProxy create new proxy
func NewProxy(listen string, timeout, shutdownTimeout time.Duration, upstream string, enableCompression bool, header http.Header, gr token.Generator) (*Proxy, error) {
	addr, err := net.ResolveTCPAddr("tcp", listen)
	if err != nil {
		return nil, err
	}
	server, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Proxy{
		server:            server,
		listen:            listen,
		timeout:           timeout,
		shutdownTimeout:   shutdownTimeout,
		upstream:          upstream,
		enableCompression: enableCompression,
		header:            header,
		gr:                gr,
		done:              make(chan struct{}),
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

func (p *Proxy) connectWS(ctx context.Context) (*websocket.Conn, error) {
	wsURL := wsRegexp.ReplaceAllString(p.upstream, "ws")
	// log.Printf("connecting to %s", wsURL)

	h2 := make(http.Header)
	nv := 0
	for _, vv := range p.header {
		nv += len(vv)
	}
	sv := make([]string, nv)
	for k, vv := range p.header {
		n := copy(sv, vv)
		h2[k] = sv[:n:n]
		sv = sv[n:]
	}

	usURL, pErr := url.Parse(p.upstream)
	if pErr != nil {
		return nil, fmt.Errorf("Failed to parse upstream url: %v", pErr)
	}
	h2.Add("Origin", usURL.Scheme+"://"+usURL.Host)

	if p.gr.Enabled() {
		t, tErr := p.gr.Get(ctx)
		if tErr != nil {
			return nil, fmt.Errorf("Failed to generate token: %v", tErr)
		}
		h2.Add("Authorization", fmt.Sprintf("Bearer %s", t))
	}

	dialer := &websocket.Dialer{
		HandshakeTimeout:  p.timeout,
		EnableCompression: p.enableCompression,
	}
	conn, _, err := dialer.Dial(wsURL, h2)
	if err != nil {
		return nil, fmt.Errorf("Dial to %s fail: %v", p.upstream, err)
	}

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
		b := pool.Get().([]byte)
		defer pool.Put(b)
		for {
			n, err := c.Read(b)
			if err != nil {
				if !goClose {
					log.Printf("Copy from client: %v", err)
				}
				return
			}
			if err := s.WriteMessage(websocket.BinaryMessage, b[:n]); err != nil {
				if !goClose {
					log.Printf("Copy from client: %v", err)
				}
				return
			}
		}
	}()

	// upstream => client
	go func() {
		defer func() { doneCh <- true }()
		b := pool.Get().([]byte)
		defer pool.Put(b)
		for {
			mt, r, err := s.NextReader()
			if err != nil {
				if !goClose {
					log.Printf("Copy from upstream: %v", err)
				}
				return
			}
			if mt != websocket.BinaryMessage {
				log.Printf("Copy from upstream: BinaryMessage required")
				return
			}
			if _, err := io.CopyBuffer(c, r, b); err != nil {
				if !goClose {
					log.Printf("Copy from upstream: %v", err)
				}
				return
			}
		}
	}()

	<-doneCh
	goClose = true
	s.Close()
	c.Close()
	<-doneCh
	return nil
}
