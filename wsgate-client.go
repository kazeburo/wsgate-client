package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	flags "github.com/jessevdk/go-flags"
	"github.com/kazeburo/wsgate-client/defaults"
	iap "github.com/kazeburo/wsgate-client/iap"
	privatekey "github.com/kazeburo/wsgate-client/privatekey"
	proxy "github.com/kazeburo/wsgate-client/proxy"
	"github.com/kazeburo/wsgate-client/token"
	"golang.org/x/sync/errgroup"
)

var (
	// Version set in compile
	Version string
	// Mapping listen => Proxy
	Mapping map[string]*proxy.Proxy
)

type cmdOpts struct {
	MapFile           string        `long:"map" description:"listen port and upstream url mapping file" required:"true"`
	ConnectTimeout    time.Duration `long:"connect-timeout" default:"60s" description:"timeout of connection to upstream"`
	Version           bool          `short:"v" long:"version" description:"Show version"`
	Headers           []string      `shrot:"H" long:"headers" description:"Header key and value added to upsteam"`
	PrivateKeyFile    string        `long:"private-key" description:"private key for signing JWT auth header"`
	PrivateKeyUser    string        `long:"private-key-user" default:"private-key-user" description:"user id which is used as subject in JWT payload"`
	IapCredentialFile string        `long:"iap-credential" description:"GCP service account json file for using wsgate -server behind IAP enabled Cloud Load Balancer"`
	IapClientID       string        `long:"iap-client-id" description:"IAP's OAuth2 Client ID"`
}

func main() {
	opts := cmdOpts{}
	psr := flags.NewParser(&opts, flags.Default)
	_, err := psr.Parse()
	if err != nil {
		os.Exit(1)
	}

	if opts.Version {
		fmt.Printf(`wsgate-client %s
Compiler: %s %s
`,
			Version,
			runtime.Compiler,
			runtime.Version())
		return
	}

	if opts.IapCredentialFile != "" && opts.PrivateKeyFile != "" {
		log.Fatalf("IapCredentialFile and PrivateKeyFile are exclusive")
	}

	ctx := context.Background()
	eg, ctx := errgroup.WithContext(ctx)
	ctx, cancel := context.WithCancel(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)
	eg.Go(func() error {
		select {
		case <-sigChan:
			cancel()
			return nil
		}
	})

	defer cancel()

	var gr token.Generator = defaults.NewGenerator()
	if opts.IapCredentialFile != "" {
		if opts.IapClientID == "" {
			log.Fatalf("IapClientID is required")
		}
		gr, err = iap.NewGenerator(opts.IapCredentialFile, opts.IapClientID)
		if err != nil {
			log.Fatalf("Failed to init iap token generator: %v", err)
		}
	} else if opts.PrivateKeyFile != "" {
		gr, err = privatekey.NewGenerator(opts.PrivateKeyFile, opts.PrivateKeyUser)
		if err != nil {
			log.Fatalf("Failed to init token generator: %v", err)
		}
	}

	if gr.Enabled() {
		_, err = gr.Get(ctx)
		if err != nil {
			log.Fatalf("Failed to get token: %v", err)
		}
		go gr.Run(ctx)
	}

	headerRegexp := regexp.MustCompile(`^(.+?):\s*(.+)$`)
	headers := http.Header{}
	for _, header := range opts.Headers {
		h := headerRegexp.FindStringSubmatch(header)
		if len(h) != 2 {
			log.Fatalf("Invalid header in %s", header)
		}
		headers.Add(h[0], h[1])
	}

	r := regexp.MustCompile(`^ *#`)
	Mapping = make(map[string]*proxy.Proxy)
	if opts.MapFile != "" {
		f, err := os.Open(opts.MapFile)
		if err != nil {
			log.Fatal(err)
		}
		s := bufio.NewScanner(f)
		for s.Scan() {
			if r.MatchString(s.Text()) {
				continue
			}
			l := strings.SplitN(s.Text(), ",", 2)
			if len(l) != 2 {
				log.Fatalf("Invalid line in %s: %s", opts.MapFile, s.Text())
			}
			log.Printf("Create map: %s => %s", l[0], l[1])
			p, err := proxy.NewProxy(l[0], opts.ConnectTimeout, l[1], headers, gr)
			if err != nil {
				log.Fatalf("could not listen %s: %v", l[0], err)
			}
			Mapping[l[0]] = p
		}
	}

	for key := range Mapping {
		key := key
		eg.Go(func() error {
			err := Mapping[key].Start(ctx)
			if err != nil {
				cancel()
			}
			return err
		})
	}
	if err := eg.Wait(); err != nil {
		log.Fatalf("failed to start proxy: %v", err)
	}
}
