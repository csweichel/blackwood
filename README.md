# Blackwood

A daemon that watches a directory for new [viwoods](https://viwoods.com) notes files and runs configurable hooks on them. Intended to run under systemd.

## Build

```sh
make build      # produces bin/blackwood
make test
make install    # go install
```

## Usage

```sh
blackwood --config path/to/config.yaml --watch-dir /path/to/notes
```

| Flag | Description |
|------|-------------|
| `--config` | Path to the configuration file |
| `--watch-dir` | Directory to watch for new notes files |

## Systemd Setup

Install the binary, unit file, and directories:

```sh
sudo make install-service
```

Copy your configuration file:

```sh
sudo cp blackwood.example.yaml /etc/blackwood/config.yaml
# Edit as needed
sudo $EDITOR /etc/blackwood/config.yaml
```

Enable and start the service:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now blackwood
```

View logs:

```sh
journalctl -u blackwood -f
```

## License

See [LICENSE](LICENSE).
