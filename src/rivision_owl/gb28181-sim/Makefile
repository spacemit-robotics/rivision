# GB28181 Simulator Makefile

VERSION ?= 1.0.0
LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION)"

.PHONY: build build-linux-amd64 build-linux-riscv64 clean

build:
go build $(LDFLAGS) -o gb28181-sim .

build-linux-amd64:
GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o gb28181-sim-linux-amd64 .

build-linux-riscv64:
GOOS=linux GOARCH=riscv64 go build $(LDFLAGS) -o gb28181-sim-linux-riscv64 .

build-all: build-linux-amd64 build-linux-riscv64

clean:
rm -f gb28181-sim gb28181-sim-linux-*

deps:
go mod tidy
