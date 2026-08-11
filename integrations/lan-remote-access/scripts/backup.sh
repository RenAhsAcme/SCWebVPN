#!/bin/sh

set -eu
umask 077

destination=${1:-/root}
stamp=$(date +%Y%m%d-%H%M%S)
archive=$destination/lan-remote-access-${stamp}.tar.gz
work=$(mktemp -d /tmp/lan-remote-backup.XXXXXX)
case $work in
    /tmp/lan-remote-backup.*) ;;
    *) echo "ERROR: unexpected temporary path: $work" >&2; exit 1 ;;
esac
trap 'rm -rf "$work"' EXIT HUP INT TERM

lan-remote exec -T postgres pg_dump \
    -p 55432 -U guacamole -d guacamole_db -Fc > "$work/guacamole.pgdump"
cp -a /opt/lan-remote-access/compose.yaml "$work/compose.yaml"
cp -a /opt/lan-remote-access/daemon.json "$work/daemon.json"
cp -a /opt/lan-remote-access/README.md "$work/README.md"
cp -a /opt/lan-remote-access/scripts "$work/scripts"
cp -a /opt/lan-remote-access/runtime "$work/runtime"
cp -a /opt/lan-remote-access/www "$work/www"
cp -a /opt/lan-remote-access/data/guacamole-drive "$work/guacamole-drive"
cp -a /opt/lan-remote-access/data/rustdesk "$work/rustdesk"
cp -a /opt/lan-remote-access/secrets "$work/secrets"
cp -a /etc/nginx/nginx.conf "$work/nginx.conf"
cp -a /etc/nginx/conf.d/lan-remote-access.conf "$work/nginx-site.conf"
uci export firewall > "$work/firewall.uci"
uci export nginx > "$work/nginx.uci"
tar -czf "$archive" -C "$work" .
chmod 0600 "$archive"
sha256sum "$archive"
