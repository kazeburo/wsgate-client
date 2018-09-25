package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"time"

	"golang.org/x/net/websocket"
)

const (
	bufferSize = 0xFFFF
)

// Proxy proxy struct
type Proxy struct {
	server   *net.TCPListener
	listen   string
	timeout  time.Duration
	upstream string
	header   http.Header
	done     chan struct{}
}

// NewProxy create new proxy
func NewProxy(listen string, timeout time.Duration, upstream string, header http.Header) (*Proxy, error) {
	addr, err := net.ResolveTCPAddr("tcp", listen)
	if err != nil {
		return nil, err
	}
	server, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Proxy{
		server:   server,
		listen:   listen,
		timeout:  timeout,
		upstream: upstream,
		header:   header,
		done:     make(chan struct{}),
	}, nil
}

// Start start new proxy
func (p *Proxy) Start(ctx context.Context) error {

	for {
		conn, err := p.server.AcceptTCP()
		if err != nil {
			return err
		}
		conn.SetNoDelay(true)
		go p.handleConn(ctx, conn)
	}
}

var wsRegexp = regexp.MustCompile("^http")

func (p *Proxy) connectWS(ctx context.Context) (net.Conn, error) {
	wsURL := wsRegexp.ReplaceAllString(p.upstream, "ws")
	log.Printf("connecting to %s", wsURL)
	wsConf, err := websocket.NewConfig(wsURL, p.upstream)
	if err != nil {
		return nil, fmt.Errorf("NewConfig failed: %v", err)
	}
	wsConf.Header = p.header
	wsConf.Dialer = &net.Dialer{
		Timeout:   p.timeout,
		KeepAlive: 10 * time.Second,
	}
	conn, err := websocket.DialConfig(wsConf)
	if err != nil {
		return nil, fmt.Errorf("Dial to %q fail: %v", p.upstream, err)
	}
	conn.PayloadType = websocket.BinaryFrame
	return conn, err
}

func (p *Proxy) handleConn(ctx context.Context, c net.Conn) error {
	s, err := p.connectWS(ctx)
	if err != nil {
		log.Printf("Failed to connect backend:%v. listen:%s client:%s", err, p.listen, c.RemoteAddr().String())
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
