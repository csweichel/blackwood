BINARY  := blackwood
OUTDIR  := bin
GOFLAGS ?=

.PHONY: build clean test install install-service

build:
	go build $(GOFLAGS) -o $(OUTDIR)/$(BINARY) ./cmd/blackwood

clean:
	rm -rf $(OUTDIR)

test:
	go test ./...

install:
	go install ./cmd/blackwood

install-service: build
	install -Dm755 bin/blackwood /usr/local/bin/blackwood
	install -Dm644 dist/blackwood.service /etc/systemd/system/blackwood.service
	install -dm755 /etc/blackwood
	install -dm755 /var/lib/blackwood
	@echo "Installed. Copy your config to /etc/blackwood/config.yaml"
	@echo "Then: systemctl daemon-reload && systemctl enable --now blackwood"
