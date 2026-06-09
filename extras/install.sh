#!/bin/sh
set -eu

repo="${BLACKWOOD_REPO:-csweichel/blackwood}"
github_base="${BLACKWOOD_GITHUB_BASE:-https://github.com}"
install_dir="${BLACKWOOD_INSTALL_DIR:-$HOME/.local/bin}"
config_dir="${BLACKWOOD_CONFIG_DIR:-$HOME/.blackwood}"
state_dir="${BLACKWOOD_STATE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/blackwood-updater}"
systemd_user_dir="${BLACKWOOD_SYSTEMD_USER_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user}"

require() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "$1 is required" >&2
		exit 1
	fi
}

require awk
require curl
require install
require tar
require uname

case "$(uname -s)" in
	Linux) os="linux" ;;
	*) echo "blackwood's one-line installer currently supports Linux user systemd installs" >&2; exit 1 ;;
esac

case "$(uname -m)" in
	x86_64|amd64) arch="amd64" ;;
	arm64|aarch64) arch="arm64" ;;
	*) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

latest_url="$github_base/$repo/releases/latest"
effective_url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$latest_url")"
latest_tag="${effective_url##*/}"
if [ -z "$latest_tag" ] || [ "$latest_tag" = "latest" ]; then
	echo "could not resolve latest release from $latest_url" >&2
	exit 1
fi

archive="blackwood_${os}_${arch}.tar.gz"
download_base="$github_base/$repo/releases/download/$latest_tag"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

curl -fL --retry 3 --retry-delay 2 -o "$tmp/$archive" "$download_base/$archive"
curl -fL --retry 3 --retry-delay 2 -o "$tmp/checksums.txt" "$download_base/checksums.txt"

checksum="$(awk -v file="$archive" '$2 == file { print $1 }' "$tmp/checksums.txt")"
if [ -z "$checksum" ]; then
	echo "checksum for $archive not found in checksums.txt" >&2
	exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
	printf '%s  %s\n' "$checksum" "$archive" > "$tmp/checksums.expected"
	(cd "$tmp" && sha256sum -c checksums.expected)
elif command -v shasum >/dev/null 2>&1; then
	actual="$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')"
	if [ "$actual" != "$checksum" ]; then
		echo "checksum mismatch for $archive" >&2
		exit 1
	fi
else
	echo "sha256sum or shasum is required to verify $archive" >&2
	exit 1
fi

mkdir -p "$tmp/extract"
tar -xzf "$tmp/$archive" -C "$tmp/extract"

if [ ! -x "$tmp/extract/blackwood" ]; then
	echo "archive did not contain an executable blackwood binary" >&2
	exit 1
fi
if [ ! -x "$tmp/extract/extras/blackwood-update" ]; then
	echo "archive did not contain extras/blackwood-update" >&2
	exit 1
fi

mkdir -p "$install_dir" "$config_dir/secrets" "$state_dir" "$systemd_user_dir"
install -m 0755 "$tmp/extract/blackwood" "$install_dir/blackwood"
install -m 0755 "$tmp/extract/extras/blackwood-update" "$install_dir/blackwood-update"
install -m 0644 "$tmp/extract/extras/blackwood-user.service" "$systemd_user_dir/blackwood.service"
install -m 0644 "$tmp/extract/extras/blackwood-update.service" "$systemd_user_dir/blackwood-update.service"
install -m 0644 "$tmp/extract/extras/blackwood-update.timer" "$systemd_user_dir/blackwood-update.timer"
printf '%s\n' "$latest_tag" > "$state_dir/version"

if [ ! -f "$config_dir/config.yaml" ]; then
	cat > "$config_dir/config.yaml" <<'EOF'
server:
  addr: ":8080"
  data_dir: ~/.blackwood

openai:
  model: gpt-5.2
  chat_model: gpt-5.2
  embedding_model: text-embedding-3-small
EOF
fi

if command -v systemctl >/dev/null 2>&1 && systemctl --user daemon-reload; then
	systemctl --user enable --now blackwood.service blackwood-update.timer
	echo "Installed Blackwood $latest_tag and enabled the user service plus updater timer."
	echo "Open http://localhost:8080 after the service starts."
else
	echo "Installed Blackwood $latest_tag, but could not enable user systemd units."
	echo "Run: systemctl --user daemon-reload && systemctl --user enable --now blackwood.service blackwood-update.timer"
fi
