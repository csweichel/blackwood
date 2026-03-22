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

## License

See [LICENSE](LICENSE).
