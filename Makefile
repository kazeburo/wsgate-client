VERSION=0.4.0
LDFLAGS=-ldflags "-w -s -X main.Version=${VERSION}"
all: wsgate-client

.PHONY: wsgate-client

wsgate-client: wsgate-client.go
	go build $(LDFLAGS) -o wsgate-client

linux: wsgate-client.go
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o wsgate-client

check:
	go test -v ./...

proccheck:
	PATH="../wsgate-server:$(HOME)/go/bin:./:$(PATH)" prove -v -r t/

fmt:
	go fmt ./...

clean:
	rm -rf wsgate-client

