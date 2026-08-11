#!/bin/sh

set -eu

archive=/tmp/lan-remote-rustdesk-web.tar.gz
manifest=/tmp/lan-remote-rustdesk-web.sha256
portal=/tmp/lan-remote-portal.html
root=/opt/lan-remote-access
target=$root/www/web
timestamp=$(date +%Y%m%d-%H%M%S)
stage=$root/www/web.stage.$timestamp
backup=$root/www/web.before-accountless.$timestamp
portal_backup=$root/www/rdp/index.html.template.before-accountless.$timestamp

[ -f "$archive" ] || { echo "Missing $archive" >&2; exit 1; }
[ -f "$manifest" ] || { echo "Missing $manifest" >&2; exit 1; }
[ -f "$portal" ] || { echo "Missing $portal" >&2; exit 1; }
[ -d "$target" ] || { echo "Missing current target: $target" >&2; exit 1; }
[ ! -e "$stage" ] || { echo "Stage already exists: $stage" >&2; exit 1; }
[ ! -e "$backup" ] || { echo "Backup already exists: $backup" >&2; exit 1; }
grep -q '__RUSTDESK_ID__' "$portal" || { echo 'Portal template has no ID placeholder.' >&2; exit 1; }
pc_id=$(cat "$root/runtime/rustdesk-id")
case $pc_id in
    *[!0-9]*|'') echo 'Stored RustDesk ID is invalid.' >&2; exit 1 ;;
esac
rustdesk_key=$(tr -d '\r\n' < "$root/data/rustdesk/id_ed25519.pub")
[ -n "$rustdesk_key" ] || { echo 'RustDesk server key is empty.' >&2; exit 1; }

mkdir "$stage"
tar -xzf "$archive" -C "$stage"
tr -d '\r' < "$manifest" > "$stage/.runtime.sha256"
(
  cd "$stage"
  sha256sum -c .runtime.sha256
)
rm "$stage/.runtime.sha256"
grep -q '__RUSTDESK_KEY__' "$stage/index.html.template" || {
  echo 'RustDesk Web template has no server key placeholder.' >&2
  exit 1
}
sed "s|__RUSTDESK_KEY__|$rustdesk_key|g" \
  "$stage/index.html.template" > "$stage/index.html"
chown -R root:root "$stage"
find "$stage" -type d -exec chmod 0755 {} \;
find "$stage" -type f -exec chmod 0644 {} \;

cp "$root/www/rdp/index.html.template" "$portal_backup"
mv "$target" "$backup"
if ! mv "$stage" "$target"; then
  mv "$backup" "$target"
  exit 1
fi

if ! cp "$portal" "$root/www/rdp/index.html.template" || \
   ! "$root/scripts/set-pc-id.sh" "$pc_id" || \
   ! nginx -t; then
  mv "$target" "$stage"
  mv "$backup" "$target"
  cp "$portal_backup" "$root/www/rdp/index.html.template"
  "$root/scripts/set-pc-id.sh" "$pc_id"
  exit 1
fi
/etc/init.d/nginx reload

printf 'Accountless RustDesk Web deployed.\nBackup: %s\nPortal backup: %s\n' "$backup" "$portal_backup"
