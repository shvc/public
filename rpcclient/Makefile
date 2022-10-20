APP?=$(shell basename ${CURDIR})
BUILDDATE=$(shell date +'%Y-%m-%dT%H:%M:%SZ')
VERSION=1.0
LONGVER=${VERSION}@${BUILDDATE}@$(shell git rev-parse --short HEAD)

LDFLAGS=-ldflags "-s -w -X main.version=${LONGVER}"

REGISTRY='myshare.io:5000'

.DEFAULT_GOAL:=default

## cover: runs go test -cover with default values
.PHONY: cover
cover:
	go test -cover ./...

## test: runs go test with default values
.PHONY: test
test:
	go test ./...

## vet: runs go vet
.PHONY: vet
vet:
	go vet ./...

## install: install to /usr/local/bin/
.PHONY: install
install: default
	mv -f ${APP} /usr/local/bin/

## default: build secgw app
.PHONY: default
default:
	@echo "Building ${APP}-${VERSION}"
	go build ${LDFLAGS}

## pkg-linux: build and package the app for linux
.PHONY: pkg-linux
pkg-linux:
	@echo "Building Linux amd64 ${APP}-${VERSION}"
	GOOS=linux GOARCH=amd64 go build ${LDFLAGS}
	zip -m ${APP}-${VERSION}-linux.zip ${APP}

## pkg: build and package the app
.PHONY: pkg
pkg:
	@echo "Building Linux amd64 ${APP}-${VERSION}"
	GOOS=linux GOARCH=amd64 go build ${LDFLAGS}
	zip -m ${APP}-${VERSION}-linux.zip ${APP}
	
	@echo "Building Macos amd64 ${APP}-${VERSION}"
	GOOS=darwin GOARCH=amd64 go build ${LDFLAGS}
	zip -m ${APP}-${VERSION}-macos.zip ${APP}
	
	@echo "Building Windows amd64 ${APP}-${VERSION}"
	GOOS=windows GOARCH=amd64 go build ${LDFLAGS}
	zip -m ${APP}-${VERSION}-win.zip ${APP}.exe

## clean: cleans the build results
.PHONY: clean
clean:
	go clean
	rm -rf *.zip  ${APP}

## image: build docker image
.PHONY: image
image:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a ${LDFLAGS}
	docker build -t shvc/${APP}:${VERSION} -f Dockerfile .

## help: prints this help message
.PHONY: help
help:
	@echo "Usage: \n"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'
