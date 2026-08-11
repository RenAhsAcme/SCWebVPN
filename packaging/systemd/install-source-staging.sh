#!/bin/sh
set -eu

usage() {
    echo "usage: install-source-staging.sh <version> <archive> <sha256>" >&2
    exit 2
}

[ "$#" -eq 3 ] || usage
version=$1
archive=$2
expected_sha256=$3

case "$version" in
    ''|*[!A-Za-z0-9._-]*) usage ;;
esac
case "$expected_sha256" in
    *[!0-9A-Fa-f]*|'') usage ;;
esac
[ "${#expected_sha256}" -eq 64 ] || usage
[ -f "$archive" ] || usage

umask 027
release_root=/opt/scwebvpn
releases=$release_root/releases
target=$releases/$version

[ ! -e "$target" ] || {
    echo "release already exists: $target" >&2
    exit 1
}

printf '%s  %s\n' "$expected_sha256" "$archive" | sha256sum -c -
install -d -o root -g root -m 0755 "$release_root" "$releases"
stage=$(mktemp -d "$release_root/.stage-$version-XXXXXX")
cleanup() {
    case "${stage:-}" in
        "$release_root"/.stage-*) rm -rf -- "$stage" ;;
    esac
}
trap cleanup EXIT INT TERM

tar -xzf "$archive" -C "$stage"
(cd "$stage" && sha256sum -c manifest.sha256)
[ -x "$stage/webvpn-controller" ] || chmod 0755 "$stage/webvpn-controller"
[ -x "$stage/authelia" ] || chmod 0755 "$stage/authelia"
[ -f "$stage/web/index.html" ] || {
    echo "browser release is incomplete" >&2
    exit 1
}
"$stage/authelia" --version

getent group webvpn >/dev/null || groupadd --system webvpn
id -u webvpn >/dev/null 2>&1 || useradd --system --gid webvpn --home-dir /nonexistent --shell /usr/sbin/nologin webvpn
getent group authelia >/dev/null || groupadd --system authelia
id -u authelia >/dev/null 2>&1 || useradd --system --gid authelia --home-dir /nonexistent --shell /usr/sbin/nologin authelia

install -d -o root -g webvpn -m 0750 /etc/scwebvpn
install -d -o webvpn -g webvpn -m 0750 /var/lib/scwebvpn
install -d -o root -g authelia -m 0750 /etc/authelia /etc/authelia/secrets
install -d -o authelia -g authelia -m 0750 /var/lib/authelia

if [ ! -e /etc/scwebvpn/controller.json ]; then
    install -o root -g webvpn -m 0640 "$stage/config-examples/controller.example.json" /etc/scwebvpn/controller.json
fi
if [ ! -e /etc/scwebvpn/internal-auth ]; then
    install -o root -g webvpn -m 0640 /dev/null /etc/scwebvpn/internal-auth
fi
if [ ! -e /etc/authelia/configuration.yml ]; then
    install -o root -g authelia -m 0640 "$stage/config-examples/authelia.configuration.yml.example" /etc/authelia/configuration.yml
fi
for secret in session storage-encryption reset-password-jwt; do
    if [ ! -e "/etc/authelia/secrets/$secret" ]; then
        install -o root -g authelia -m 0640 /dev/null "/etc/authelia/secrets/$secret"
    fi
done

chown -R root:root "$stage"
find "$stage" -type d -exec chmod 0755 {} +
find "$stage" -type f -exec chmod 0644 {} +
chmod 0755 "$stage/webvpn-controller" "$stage/authelia" "$stage/systemd/install-source-staging.sh"
mv "$stage" "$target"
stage=

install -o root -g root -m 0644 "$target/systemd/scwebvpn-controller.service" /etc/systemd/system/scwebvpn-controller.service
install -o root -g root -m 0644 "$target/systemd/scwebvpn-authelia.service" /etc/systemd/system/scwebvpn-authelia.service
if [ ! -e "$release_root/current" ]; then
    ln -s "releases/$version" "$release_root/current"
fi

systemctl daemon-reload
systemd-analyze verify /etc/systemd/system/scwebvpn-controller.service /etc/systemd/system/scwebvpn-authelia.service

echo "STAGED_RELEASE=$target"
echo "CURRENT_RELEASE=$(readlink -f "$release_root/current")"
echo "SERVICES_NOT_STARTED"
