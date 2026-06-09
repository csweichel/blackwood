BINARY  := blackwood
OUTDIR  := bin
GOFLAGS ?=

.PHONY: build build-server generate clean test install install-service install-user-service

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
	install -Dm644 extras/blackwood.service /etc/systemd/system/blackwood.service
	install -dm755 /etc/blackwood
	install -dm755 /var/lib/blackwood
	@echo "Installed. Copy your config to /etc/blackwood/config.yaml"
	@echo "Then: systemctl daemon-reload && systemctl enable --now blackwood"

install-user-service: build
	install -Dm755 bin/blackwood $(HOME)/.local/bin/blackwood
	install -Dm755 extras/blackwood-update $(HOME)/.local/bin/blackwood-update
	install -Dm644 extras/blackwood-user.service $(HOME)/.config/systemd/user/blackwood.service
	install -Dm644 extras/blackwood-update.service $(HOME)/.config/systemd/user/blackwood-update.service
	install -Dm644 extras/blackwood-update.timer $(HOME)/.config/systemd/user/blackwood-update.timer
	systemctl --user daemon-reload
	@echo "Installed user services. Copy your config to $(HOME)/.blackwood/config.yaml"
	@echo "Then: systemctl --user enable --now blackwood.service blackwood-update.timer"

.PHONY: web-install
web-install:
	cd web && npm install

.PHONY: web-build
web-build:
	cd web && npm run build

.PHONY: desktop-install
desktop-install:
	cd electron && npm ci

.PHONY: desktop-build
desktop-build:
	cd electron && npx electron-builder --mac --x64 --arm64
