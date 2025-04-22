VERSION=0.4.1
LDFLAGS=-ldflags "-w -s -X main.Version=${VERSION}"
all: wsgate-client

.PHONY: wsgate-client

wsgate-client: cmd/wsgate-client/main.go
	go build $(LDFLAGS) -o wsgate-client cmd/wsgate-client/main.go

linux: cmd/wsgate-client/main.go
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o wsgate-client cmd/wsgate-client/main.go

check:
	go test -v ./...

proccheck:
	PATH="../wsgate-server:$(HOME)/go/bin:./:$(PATH)" prove -v -r t/

fmt:
	go fmt ./...

clean:
	rm -rf wsgate-client

