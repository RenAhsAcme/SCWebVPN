#!/bin/sh

set -eu

[ "$#" -eq 1 ] || { echo "Usage: $0 RUSTDESK_ID" >&2; exit 2; }
case "$1" in
    *[!0-9]*|'') echo 'ERROR: RustDesk ID must contain digits only.' >&2; exit 2 ;;
esac

template=/opt/lan-remote-access/www/rdp/index.html.template
output=/opt/lan-remote-access/www/rdp/index.html
state=/opt/lan-remote-access/runtime/rustdesk-id
printf '%s\n' "$1" > "$state"
chmod 0644 "$state"
sed "s|__RUSTDESK_ID__|$1|g" "$template" > "$output"
chmod 0644 "$output"
printf 'Portal now displays RustDesk ID %s.\n' "$1"
