VERSION=0.0.1
LDFLAGS=-ldflags "-X main.Version=${VERSION}"
all: wsgate-client

.PHONY: wsgate-client

bundle:
	dep ensure

update:
	dep ensure -update

wsgate-client: wsgate-client.go
	go build $(LDFLAGS) -o wsgate-client

linux: wsgate-client.go
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o wsgate-client

fmt:
	go fmt ./...

clean:
	rm -rf wsgate-client

tag:
	git tag v${VERSION}
	git push origin v${VERSION}
	git push origin master
