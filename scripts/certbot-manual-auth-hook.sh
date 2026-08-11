#!/bin/sh
set -eu

if [ "${WEBVPN_ACME_INTERACTIVE:-}" != "1" ]; then
    echo "SCWebVPN DNS-01 renewal requires an attended DNS update" >&2
    exit 3
fi

state_dir=${WEBVPN_ACME_STATE_DIR:-/run/scwebvpn-acme}
domain=${CERTBOT_DOMAIN:-}
validation=${CERTBOT_VALIDATION:-}

case "$domain" in
    ''|*[!A-Za-z0-9.*-]*) exit 2 ;;
esac
case "$validation" in
    ''|*[!A-Za-z0-9_-]*) exit 2 ;;
esac
case "$domain" in
    \*.*) record_domain=${domain#*.} ;;
    *) record_domain=$domain ;;
esac

umask 077
install -d -m 0700 "$state_dir"
current_tmp=$state_dir/current.$$
printf 'record=_acme-challenge.%s\nvalue=%s\n' "$record_domain" "$validation" >"$current_tmp"
mv "$current_tmp" "$state_dir/current"

ready=$state_dir/ready-$validation
attempt=0
while [ ! -e "$ready" ]; do
    attempt=$((attempt + 1))
    [ "$attempt" -le 3600 ] || {
        echo "DNS-01 challenge confirmation timed out" >&2
        exit 1
    }
    sleep 2
done
rm -f "$ready" "$state_dir/current"
