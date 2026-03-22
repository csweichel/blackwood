BINARY  := blackwood
OUTDIR  := bin
GOFLAGS ?=

.PHONY: build clean test install

build:
	go build $(GOFLAGS) -o $(OUTDIR)/$(BINARY) ./cmd/blackwood

clean:
	rm -rf $(OUTDIR)

test:
	go test ./...

install:
	go install ./cmd/blackwood
