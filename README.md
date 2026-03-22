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

## Dropbox Integration

Blackwood can monitor a Dropbox-synced folder for new notes. This requires the [Dropbox desktop client](https://www.dropbox.com/install) to be installed and syncing on the same machine.

Instead of setting `watch_dir`, use `dropbox.local_path` to point at your locally-synced Dropbox folder:

```yaml
dropbox:
  local_path: ~/Dropbox/Apps/Viwoods
```

Blackwood will use this path as the watch directory automatically. Note that `watch_dir` and `dropbox.local_path` are mutually exclusive — setting both is an error.

## License

See [LICENSE](LICENSE).
