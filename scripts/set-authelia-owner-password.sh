#!/bin/bash
set -euo pipefail

bin=/opt/scwebvpn/current/authelia
config=/etc/authelia/configuration.yml
users=/etc/authelia/users_database.yml

if [[ $EUID -ne 0 || ! -t 0 || ! -t 1 ]]; then
    echo 'Run this script as root from an interactive TTY.' >&2
    exit 2
fi
[[ -x $bin && -f $config && -f $users ]]

echo 'Authelia will request the WebVPN owner password twice. Input is hidden.'
transcript=$(mktemp /root/.webvpn-password-output.XXXXXX)
tmp=
cleanup() {
    rm -f "${transcript:-}" "${tmp:-}"
}
trap cleanup EXIT INT TERM
chmod 0600 "$transcript"
script -q -e -E auto -o 64K -c "$bin crypto hash generate argon2" "$transcript"
digest=$(tr -d '\r' <"$transcript" | sed -n 's/^Digest: //p' | tail -n 1)
case "$digest" in
    '$argon2id$'*) ;;
    *) echo 'Authelia did not return an Argon2id digest.' >&2; exit 1 ;;
esac
rm -f "$transcript"
transcript=

stamp=$(date +%Y%m%d-%H%M%S)
backup=/root/webvpn-backups/authelia-owner-$stamp
install -d -m 0700 "$backup"
cp -p "$users" "$backup/users_database.yml"

tmp=$(mktemp /etc/authelia/.users.XXXXXX)
umask 077
printf '%s\n' \
    'users:' \
    '  owner:' \
    '    disabled: false' \
    '    displayname: Owner' \
    "    password: '$digest'" \
    '    email: owner@localhost' \
    '    groups:' \
    '      - webvpn' >"$tmp"
unset digest
chown root:authelia "$tmp"
chmod 0640 "$tmp"
mv "$tmp" "$users"
tmp=

if ! env \
    AUTHELIA_SESSION_SECRET_FILE=/etc/authelia/secrets/session \
    AUTHELIA_STORAGE_ENCRYPTION_KEY_FILE=/etc/authelia/secrets/storage-encryption \
    AUTHELIA_IDENTITY_VALIDATION_RESET_PASSWORD_JWT_SECRET_FILE=/etc/authelia/secrets/reset-password-jwt \
    "$bin" config validate --config "$config"; then
    cp -p "$backup/users_database.yml" "$users"
    exit 1
fi

if ! systemctl restart scwebvpn-authelia.service; then
    cp -p "$backup/users_database.yml" "$users"
    systemctl restart scwebvpn-authelia.service || true
    exit 1
fi
systemctl is-active --quiet scwebvpn-authelia.service
echo "Owner password updated. Backup: $backup"
