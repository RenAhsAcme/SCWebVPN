#!/bin/sh

set -eu

pc_address=$(cat /opt/lan-remote-access/runtime/pc-lan-address)
case $pc_address in
    192.168.1.*|192.168.3.*) ;;
    *) echo 'ERROR: invalid PC LAN address.' >&2; exit 1 ;;
esac

echo 'Container state'
lan-remote ps

for service in postgres guacd guacamole hbbs hbbr; do
    container=$(lan-remote ps -q "$service")
    [ -n "$container" ] || { echo "ERROR: $service container is missing." >&2; exit 1; }
    state=$(/opt/lan-remote-access/bin/docker \
        --host unix:///var/run/lan-remote/docker.sock \
        inspect --format '{{.State.Status}}' "$container")
    [ "$state" = running ] || { echo "ERROR: $service is $state." >&2; exit 1; }
    health=$(/opt/lan-remote-access/bin/docker \
        --host unix:///var/run/lan-remote/docker.sock \
        inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container")
    [ "$health" = none ] || [ "$health" = healthy ] || {
        echo "ERROR: $service health is $health." >&2
        exit 1
    }
done

echo
echo 'Loopback-only internals'
netstat -lntp 2>/dev/null | grep -E '(::ffff:)?127\.0\.0\.1:(8080|54822|55432)' || true

echo
echo 'WebVPN loopback gateway'
netstat -lntp 2>/dev/null | grep '127\.0\.0\.1:18081' || {
    echo 'ERROR: WebVPN loopback gateway is unavailable.' >&2
    exit 1
}

echo
echo 'PC file channel'
file_token=$(wget -q -T 5 -O - \
    --header='X-WebVPN-User: webvpn' \
    --header='Content-Type: application/json' \
    --post-data='{}' \
    "http://$pc_address:18080/files/api/login") || {
    echo 'ERROR: PC file channel is unavailable.' >&2
    exit 1
}
case $file_token in
    *.*.*) ;;
    *) echo 'ERROR: PC file channel did not issue a session token.' >&2; exit 1 ;;
esac
wget -q -T 5 -O /dev/null \
    --header="X-Auth: $file_token" \
    "http://$pc_address:18080/files/api/resources/" || {
    echo 'ERROR: PC file channel authorization failed.' >&2
    exit 1
}
unset file_token

echo
echo 'Configuration checks'
nginx -t -c /etc/nginx/nginx.conf
[ ! -d /sys/class/net/docker0 ] || { echo 'ERROR: unexpected docker0 bridge exists.' >&2; exit 1; }
nft --stateless list ruleset | sha256sum
