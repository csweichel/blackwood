BINARY  := blackwood
OUTDIR  := bin
GOFLAGS ?=

.PHONY: build build-server generate clean test install install-service

generate:
	buf generate

build:
	go build $(GOFLAGS) -o $(OUTDIR)/$(BINARY) ./cmd/blackwood

build-server: generate
	go build $(GOFLAGS) -o $(OUTDIR)/blackwood ./cmd/blackwood

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

.PHONY: web-install
web-install:
	cd web && npm install

.PHONY: web-build
web-build:
	cd web && npm run build
