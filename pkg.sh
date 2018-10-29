#!/bin/sh

Version="1.0.$(git rev-list --all --count)"

ServerURL='http://www.aweg.cc:23456/ping'
if [ "X$1" != "X" ]
then
  ServerURL=$1
fi

echo "Building linux arm64 ..."
GOOS=linux GOARCH=arm64 go build -ldflags "-X main.Version=$Version -X main.ServerURL=$ServerURL"
zip -m upnp-$Version-linux-arm64.zip upnp

echo "Building linux amd64 ..."
GOOS=linux GOARCH=amd64 go build -ldflags "-X main.Version=$Version -X main.ServerURL=$ServerURL"
zip -m upnp-$Version-linux-amd64.zip upnp

echo "Building windows x64 ..."
GOOS=windows GOARCH=amd64 go build -ldflags "-X main.Version=$Version -X main.ServerURL=$ServerURL"
zip -m upnp-$Version-win64.zip upnp.exe

echo "Building windows x86 ..."
GOOS=windows GOARCH=386 go build -ldflags "-X main.Version=$Version -X main.ServerURL=$ServerURL"
zip -m upnp-$Version-x86.zip upnp.exe

echo "Building MacOS x64 ..."
GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.Version=$Version -X main.ServerURL=$ServerURL"
zip -m upnp-$Version-mac64.zip upnp

