#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/povr-helper.XXXXXX")"
bundle_path="$work_dir/Povr.app"
replacement_path=""
backup_path=""
install_path=""

cleanup() {
	rm -rf "$work_dir"
	if [[ -n "$replacement_path" ]]; then
		rm -rf "$replacement_path"
	fi
	if [[ -n "$backup_path" && -e "$backup_path" && -n "$install_path" && ! -e "$install_path" ]]; then
		mv "$backup_path" "$install_path"
	fi
}
trap cleanup EXIT

if [[ -z "${HOME:-}" || "$HOME" == "/" ]]; then
	echo "error: HOME must name a user home directory" >&2
	exit 1
fi

echo "assembling Povr.app bundle"
mkdir -p "$bundle_path/Contents/MacOS" "$bundle_path/Contents/Resources"
cp "$repo_root/scripts/Info.plist" "$bundle_path/Contents/Info.plist"

echo "building povr-helper"
(
	cd "$repo_root"
	CGO_ENABLED=1 go build -o "$bundle_path/Contents/MacOS/povr-helper" ./cmd/povr-helper
)

echo "signing Povr.app"
codesign -s - \
	--entitlements "$repo_root/scripts/helper-entitlements.plist" \
	--force \
	"$bundle_path"

install_parent="$HOME/Library/Application Support/povr"
install_path="$install_parent/Povr.app"
replacement_path="$install_parent/.Povr.app.new.$$"
backup_path="$install_parent/.Povr.app.old.$$"

echo "staging Povr.app installation"
mkdir -p "$install_parent"
cp -R "$bundle_path" "$replacement_path"

if [[ -e "$install_path" ]]; then
	mv "$install_path" "$backup_path"
fi
if mv "$replacement_path" "$install_path"; then
	replacement_path=""
	rm -rf "$backup_path"
	backup_path=""
else
	if [[ -e "$backup_path" ]]; then
		mv "$backup_path" "$install_path"
	fi
	exit 1
fi

echo "installed Povr.app at $install_path"
