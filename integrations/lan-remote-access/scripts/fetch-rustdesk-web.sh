#!/bin/sh

set -eu
umask 022

root=/opt/lan-remote-access
base=https://rustdesk.com/web
manifest=$root/runtime/rustdesk-web-runtime.sha256
work=$(mktemp -d /tmp/lan-remote-web.XXXXXX)

case $work in
    /tmp/lan-remote-web.*) ;;
    *) echo "ERROR: unexpected temporary path: $work" >&2; exit 1 ;;
esac
trap 'rm -rf "$work"' EXIT HUP INT TERM

count=$(wc -l < "$manifest" | tr -d ' ')
[ "$count" = 104 ] || {
    echo "ERROR: expected 104 RustDesk Web runtime files, got $count" >&2
    exit 1
}

while read -r expected path; do
    target=$root/www/web/$path
    if [ -s "$target" ] && [ "$(sha256sum "$target" | awk '{print $1}')" = "$expected" ]; then
        continue
    fi

    mkdir -p "$(dirname "$target")"
    wget -q -O "$work/file" "$base/$path"
    actual=$(sha256sum "$work/file" | awk '{print $1}')
    [ "$actual" = "$expected" ] || {
        echo "ERROR: RustDesk Web hash mismatch for $path" >&2
        exit 1
    }
    mv "$work/file" "$target"
done < "$manifest"

echo "Verified $count pinned RustDesk Web runtime files."
