#!/bin/sh
# Offline install -> upgrade -> dry-run uninstall -> uninstall smoke test.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT HUP INT TERM

FIXTURE="$TMP/fixture"
FAKEBIN="$TMP/fakebin"
INSTALL_DIR="$TMP/bin"
HOME_DIR="$TMP/home"
mkdir -p "$FIXTURE" "$FAKEBIN" "$INSTALL_DIR" "$HOME_DIR/.novelforge" "$HOME_DIR/.ainovel"
printf '%s\n' 'new-config-secret-sentinel' > "$HOME_DIR/.novelforge/config.json"
printf '%s\n' 'legacy-config-secret-sentinel' > "$HOME_DIR/.ainovel/config.json"

if command -v sha256sum >/dev/null 2>&1; then
	sha256_file() { sha256sum "$1" | awk '{print tolower($1)}'; }
elif command -v shasum >/dev/null 2>&1; then
	sha256_file() { shasum -a 256 "$1" | awk '{print tolower($1)}'; }
else
	echo "smoke test requires sha256sum or shasum" >&2
	exit 1
fi

cat > "$FAKEBIN/curl" <<'CURL'
#!/bin/sh
set -eu
out=""
url=""
while [ "$#" -gt 0 ]; do
	case "$1" in
		-o)
			out="$2"
			shift 2
			;;
		--proto|--proto-redir)
			shift 2
			;;
		--tlsv1.2|-f|-s|-S|-L|-fsSL)
			shift
			;;
		*)
			url="$1"
			shift
			;;
	esac
done
case "$url" in
	*/releases/latest|*/releases/tags/*)
		source="$NOVELFORGE_SMOKE_FIXTURE/release.json"
		;;
	*/novelforge_checksums.txt)
		source="$NOVELFORGE_SMOKE_FIXTURE/novelforge_checksums.txt"
		;;
	*/novelforge_*.tar.gz)
		source="$NOVELFORGE_SMOKE_FIXTURE/$(basename "$url")"
		;;
	*)
		echo "unexpected curl URL: $url" >&2
		exit 1
		;;
esac
if [ -n "$out" ]; then
	cp "$source" "$out"
else
	cat "$source"
fi
CURL
chmod +x "$FAKEBIN/curl"

make_release() {
	version="$1"
	payload="$2"
	package="$TMP/package"
	rm -rf "$package"
	mkdir -p "$package"
	cat > "$package/novelforge" <<SCRIPT
#!/bin/sh
printf '%s\\n' '$payload'
SCRIPT
	chmod 0755 "$package/novelforge"
	asset="novelforge_${version}_Linux_x86_64.tar.gz"
	tar -C "$package" -czf "$FIXTURE/$asset" novelforge
	digest=$(sha256_file "$FIXTURE/$asset")
	printf '%s  %s\n' "$digest" "$asset" > "$FIXTURE/novelforge_checksums.txt"
	printf '{"tag_name":"v%s"}\n' "$version" > "$FIXTURE/release.json"
}

run_installer() {
	version="$1"
	PATH="$FAKEBIN:$PATH" \
	HOME="$HOME_DIR" \
	USERPROFILE="$HOME_DIR" \
	NOVELFORGE_SMOKE_FIXTURE="$FIXTURE" \
	NOVELFORGE_INSTALL_DIR="$INSTALL_DIR" \
	NOVELFORGE_VERSION="v$version" \
	sh "$ROOT/scripts/install.sh" >/dev/null
}

make_release "0.1.0" "phase1-install-v1"
run_installer "0.1.0"
[ -x "$INSTALL_DIR/novelforge" ]
[ "$("$INSTALL_DIR/novelforge")" = "phase1-install-v1" ]

make_release "0.1.1" "phase1-upgrade-v2"
run_installer "0.1.1"
[ "$("$INSTALL_DIR/novelforge")" = "phase1-upgrade-v2" ]

NOVELFORGE_INSTALL_DIR="$INSTALL_DIR" sh "$ROOT/scripts/uninstall.sh" --dry-run >/dev/null
[ -x "$INSTALL_DIR/novelforge" ]
NOVELFORGE_INSTALL_DIR="$INSTALL_DIR" sh "$ROOT/scripts/uninstall.sh" >/dev/null
[ ! -e "$INSTALL_DIR/novelforge" ]

[ "$(cat "$HOME_DIR/.novelforge/config.json")" = "new-config-secret-sentinel" ]
[ "$(cat "$HOME_DIR/.ainovel/config.json")" = "legacy-config-secret-sentinel" ]

echo "NovelForge install/upgrade/uninstall smoke: PASS"
