package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	flags "github.com/jessevdk/go-flags"
	proxy "github.com/kazeburo/wsgate-client/proxy"
	"golang.org/x/sync/errgroup"
)

var (
	// Version set in compile
	Version string
	// Mapping listen => Proxy
	Mapping map[string]*proxy.Proxy
)

type cmdOpts struct {
	MapFile        string        `long:"map" description:"listen port and upstream url mapping file" required:"true"`
	ConnectTimeout time.Duration `long:"connect-timeout" default:"60s" description:"timeout of connection to upstream"`
	Version        bool          `short:"v" long:"version" description:"Show version"`
	Headers        []string      `shrot:"H" long:"headers" description:"Header key and value added to upsteam"`
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

	ctx := context.Background()

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
			p, err := proxy.NewProxy(l[0], opts.ConnectTimeout, l[1], headers)
			if err != nil {
				log.Fatalf("could not listen %s: %v", l[0], err)
			}
			Mapping[l[0]] = p
		}
	}

	var wg errgroup.Group
	for key := range Mapping {
		key := key
		wg.Go(func() error {
			return Mapping[key].Start(ctx)
		})
	}
	wg.Wait()
}
