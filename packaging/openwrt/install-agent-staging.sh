#!/bin/sh
set -eu

usage() {
    echo "usage: install-agent-staging.sh <version> <agent> <sha256> <init-script>" >&2
    exit 2
}

[ "$#" -eq 4 ] || usage
version=$1
agent=$2
expected_sha256=$3
init_script=$4

case "$version" in
    ''|*[!A-Za-z0-9._-]*) usage ;;
esac
case "$expected_sha256" in
    *[!0-9A-Fa-f]*|'') usage ;;
esac
[ "${#expected_sha256}" -eq 64 ] || usage
[ -f "$agent" ] || usage
[ -f "$init_script" ] || usage

uid=8789
gid=8789
root=/usr/libexec/scwebvpn
target=$root/releases/$version
config=/etc/scwebvpn
service=/etc/init.d/scwebvpn-agent

[ "$(sha256sum "$agent" | awk '{print $1}')" = "$expected_sha256" ]
[ ! -e "$target" ]
[ ! -e "$service" ]

if grep -q '^webvpn:' /etc/passwd || grep -q '^webvpn:' /etc/group; then
    [ "$(id -u webvpn)" = "$uid" ]
    [ "$(id -g webvpn)" = "$gid" ]
else
    ! awk -F: -v id="$uid" '$3 == id { found=1 } END { exit found ? 0 : 1 }' /etc/passwd
    ! awk -F: -v id="$gid" '$3 == id { found=1 } END { exit found ? 0 : 1 }' /etc/group
    ! grep -q '^webvpn:' /etc/shadow

    stamp=$(date +%Y%m%d-%H%M%S)
    backup=/root/webvpn-backups/agent-$stamp
    mkdir -p "$backup"
    chmod 0700 "$backup"
    cp -p /etc/passwd "$backup/passwd"
    cp -p /etc/group "$backup/group"
    cp -p /etc/shadow "$backup/shadow"

    umask 077
    passwd_new=$(mktemp /etc/.passwd.webvpn.XXXXXX)
    group_new=$(mktemp /etc/.group.webvpn.XXXXXX)
    shadow_new=$(mktemp /etc/.shadow.webvpn.XXXXXX)
    cp /etc/passwd "$passwd_new"
    cp /etc/group "$group_new"
    cp /etc/shadow "$shadow_new"
    printf 'webvpn:x:%s:%s:WebVPN Agent:/var/run/webvpn:/bin/false\n' "$uid" "$gid" >>"$passwd_new"
    printf 'webvpn:x:%s:\n' "$gid" >>"$group_new"
    printf 'webvpn:!:0:0:99999:7:::\n' >>"$shadow_new"
    chown root:root "$passwd_new" "$group_new" "$shadow_new"
    chmod 0644 "$passwd_new" "$group_new"
    chmod 0600 "$shadow_new"
    mv "$group_new" /etc/group
    mv "$passwd_new" /etc/passwd
    mv "$shadow_new" /etc/shadow
fi

mkdir -p "$target" "$config/ca" "$config/identity"
chmod 0755 "$root" "$root/releases" "$target"
chown root:webvpn "$config" "$config/ca"
chmod 0750 "$config" "$config/ca"
chown webvpn:webvpn "$config/identity"
chmod 0700 "$config/identity"

cp "$agent" "$target/webvpn-agent"
chown root:root "$target/webvpn-agent"
chmod 0555 "$target/webvpn-agent"
[ "$(sha256sum "$target/webvpn-agent" | awk '{print $1}')" = "$expected_sha256" ]

if [ ! -e "$root/current" ]; then
    ln -s "releases/$version" "$root/current"
fi
cp "$init_script" "$service"
chown root:root "$service"
chmod 0755 "$service"

[ ! -e /etc/rc.d/S95scwebvpn-agent ]
! pgrep -f "$root/current/webvpn-agent run" >/dev/null
printf 'STAGED_VERSION=%s\nSERVICES_NOT_STARTED\n' "$target"
