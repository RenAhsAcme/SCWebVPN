#!/bin/sh

set -eu
umask 077

SOURCE_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TARGET_DIR=/opt/lan-remote-access
DOCKER_VERSION=29.7.1
DOCKER_SHA256=0fcea2a8b4d1b54ccc9010e3451b78504a369d414f37eb3bb79300e1b5c22ce6
COMPOSE_VERSION=5.4.0
COMPOSE_SHA256=837fd1d35bf6a494f41b5b5988269a7be79de337cf1a1a6ff0e45ab51bb4e9be
COMPOSE_CLI_IMAGE=docker@sha256:27a51d5ab1cd38d9eeaba7b415b8c07bc10c31e1cf1ec8d78f6413fcfab3f44f
SOCKET=unix:///var/run/lan-remote/docker.sock
RUNTIME_ARCHIVE=/tmp/lan-remote-docker-${DOCKER_VERSION}.tgz
RUNTIME_EXTRACT=/tmp/lan-remote-docker-${DOCKER_VERSION}

usage() {
    echo "usage: install-openwrt.sh <pc-lan-address>" >&2
    exit 2
}

[ "$#" -eq 1 ] || usage
PC_ADDRESS=$1
case $PC_ADDRESS in
    192.168.1.*|192.168.3.*) ;;
    *) usage ;;
esac
PC_NETWORK=${PC_ADDRESS%.*}
PC_LAST=${PC_ADDRESS##*.}
case $PC_LAST in
    ''|*[!0-9]*) usage ;;
esac
[ "$PC_LAST" -ge 1 ] && [ "$PC_LAST" -le 254 ] && [ "$PC_ADDRESS" = "$PC_NETWORK.$PC_LAST" ] || usage

log() {
    printf '\n==> %s\n' "$*"
}

require_value() {
    actual=$1
    expected=$2
    label=$3
    if [ "$actual" != "$expected" ]; then
        printf 'ERROR: %s mismatch: expected %s, got %s\n' "$label" "$expected" "$actual" >&2
        exit 1
    fi
}

wait_for_socket() {
    attempt=0
    while [ ! -S /var/run/lan-remote/docker.sock ]; do
        attempt=$((attempt + 1))
        [ "$attempt" -lt 31 ] || return 1
        sleep 1
    done
}

configure_firewall() {
    uci -q delete firewall.lan_remote_ssh || true
    uci -q delete firewall.lan_remote_https || true

    uci -q delete firewall.lan_remote_rustdesk_tcp || true
    uci set firewall.lan_remote_rustdesk_tcp=rule
    uci set firewall.lan_remote_rustdesk_tcp.name='WebVPN RustDesk TCP from PC'
    uci set firewall.lan_remote_rustdesk_tcp.src='lan'
    uci set firewall.lan_remote_rustdesk_tcp.src_ip="$PC_ADDRESS"
    uci set firewall.lan_remote_rustdesk_tcp.proto='tcp'
    uci set firewall.lan_remote_rustdesk_tcp.dest_port='21115-21117'
    uci set firewall.lan_remote_rustdesk_tcp.family='ipv4'
    uci set firewall.lan_remote_rustdesk_tcp.target='ACCEPT'

    uci -q delete firewall.lan_remote_rustdesk_udp || true
    uci set firewall.lan_remote_rustdesk_udp=rule
    uci set firewall.lan_remote_rustdesk_udp.name='WebVPN RustDesk UDP from PC'
    uci set firewall.lan_remote_rustdesk_udp.src='lan'
    uci set firewall.lan_remote_rustdesk_udp.src_ip="$PC_ADDRESS"
    uci set firewall.lan_remote_rustdesk_udp.proto='udp'
    uci set firewall.lan_remote_rustdesk_udp.dest_port='21116'
    uci set firewall.lan_remote_rustdesk_udp.family='ipv4'
    uci set firewall.lan_remote_rustdesk_udp.target='ACCEPT'

    uci -q delete firewall.lan_remote_lan_reject || true
    uci set firewall.lan_remote_lan_reject=rule
    uci set firewall.lan_remote_lan_reject.name='Block SCWebVPN Remote internals from LAN'
    uci set firewall.lan_remote_lan_reject.src='lan'
    uci set firewall.lan_remote_lan_reject.proto='tcp udp'
    uci set firewall.lan_remote_lan_reject.dest_port='8080 54822 55432 21115-21119'
    uci set firewall.lan_remote_lan_reject.family='ipv4'
    uci set firewall.lan_remote_lan_reject.target='REJECT'

    uci commit firewall
    /etc/init.d/firewall reload
}

[ "$(uname -m)" = x86_64 ] || { echo 'ERROR: this bundle targets x86_64 OpenWrt.' >&2; exit 1; }
case $PC_ADDRESS in
    192.168.1.*)
        ip route get "$PC_ADDRESS" | grep -q "^$PC_ADDRESS dev br-lan" || {
            echo 'ERROR: the PC address is not directly reachable through br-lan.' >&2
            exit 1
        }
        ;;
    192.168.3.*)
        ip route get "$PC_ADDRESS" | grep -q 'via 192\.168\.1\.173 dev br-lan' || {
            echo 'ERROR: the PC address is not routed through the approved downstream gateway.' >&2
            exit 1
        }
        ;;
esac
grep -q '[[:space:]]overlay$' /proc/filesystems || {
    echo 'ERROR: OverlayFS is unavailable in this kernel.' >&2
    exit 1
}

log 'Backing up the current target and router configuration'
stamp=$(date +%Y%m%d-%H%M%S)
backup_dir=/tmp/lan-remote-backup-${stamp}
mkdir -p "$backup_dir"
cp -a /etc/config/firewall "$backup_dir/firewall"
cp -a /etc/config/uhttpd "$backup_dir/uhttpd"
[ ! -f /etc/config/nginx ] || cp -a /etc/config/nginx "$backup_dir/nginx"
if [ -d "$TARGET_DIR" ]; then
    mkdir -p "$backup_dir/lan-remote-access"
    for item in compose.yaml daemon.json README.md scripts runtime secrets www; do
        [ ! -e "$TARGET_DIR/$item" ] || cp -a "$TARGET_DIR/$item" "$backup_dir/lan-remote-access/$item"
    done
    [ ! -d "$TARGET_DIR/data/rustdesk" ] || \
        cp -a "$TARGET_DIR/data/rustdesk" "$backup_dir/lan-remote-access/rustdesk"
fi
tar -czf "/root/lan-remote-access-preinstall-${stamp}.tar.gz" -C "$backup_dir" .

log 'Installing the OpenWrt-native loopback gateway packages'
apk update
apk add nginx-ssl openssl-util

log 'Installing the verified static Docker runtime without kernel packages'
if [ -f /tmp/remote-access-runtime-probe-2971/docker.tgz ]; then
    cp /tmp/remote-access-runtime-probe-2971/docker.tgz "$RUNTIME_ARCHIVE"
else
    wget -O "$RUNTIME_ARCHIVE" "https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VERSION}.tgz"
fi
runtime_hash=$(sha256sum "$RUNTIME_ARCHIVE" | awk '{print $1}')
require_value "$runtime_hash" "$DOCKER_SHA256" 'Docker archive SHA-256'

rm -rf "$RUNTIME_EXTRACT"
mkdir -p "$RUNTIME_EXTRACT"
tar -xzf "$RUNTIME_ARCHIVE" -C "$RUNTIME_EXTRACT"

mkdir -p \
    "$TARGET_DIR/bin" \
    "$TARGET_DIR/cache/rustdesk-web" \
    "$TARGET_DIR/data/guacamole-drive" \
    "$TARGET_DIR/data/postgres" \
    "$TARGET_DIR/data/rustdesk" \
    "$TARGET_DIR/docker-cli/cli-plugins" \
    "$TARGET_DIR/runtime" \
    "$TARGET_DIR/scripts" \
    "$TARGET_DIR/secrets" \
    "$TARGET_DIR/www/rdp" \
    "$TARGET_DIR/www/web"
cp "$RUNTIME_EXTRACT"/docker/* "$TARGET_DIR/bin/"
chmod 0755 "$TARGET_DIR"/bin/*

cp -a "$SOURCE_DIR/compose.yaml" "$TARGET_DIR/compose.yaml"
cp -a "$SOURCE_DIR/openwrt/daemon.json" "$TARGET_DIR/daemon.json"
cp -a "$SOURCE_DIR/portal/index.html" "$TARGET_DIR/www/rdp/index.html.template"
cp -a "$SOURCE_DIR/rustdesk-web-open/." "$TARGET_DIR/www/web/"
cp -a "$SOURCE_DIR/rustdesk-web-open.sha256" "$TARGET_DIR/runtime/rustdesk-web-runtime.sha256"
sed "s/__PC_LAN_ADDRESS__/$PC_ADDRESS/g" \
    "$SOURCE_DIR/sql/bootstrap.sql" > "$TARGET_DIR/runtime/bootstrap.sql"
printf '%s\n' "$PC_ADDRESS" > "$TARGET_DIR/runtime/pc-lan-address"
cp -a "$SOURCE_DIR/README.md" "$TARGET_DIR/README.md"
cp -a "$SOURCE_DIR/scripts/backup.sh" "$TARGET_DIR/scripts/backup.sh"
cp -a "$SOURCE_DIR/scripts/install-openwrt.sh" "$TARGET_DIR/scripts/install-openwrt.sh"
cp -a "$SOURCE_DIR/scripts/set-pc-id.sh" "$TARGET_DIR/scripts/set-pc-id.sh"
cp -a "$SOURCE_DIR/scripts/status.sh" "$TARGET_DIR/scripts/status.sh"
cp -a "$SOURCE_DIR/scripts/lan-remote" /usr/sbin/lan-remote
cp -a "$SOURCE_DIR/openwrt/lan-remote-runtime" /etc/init.d/lan-remote-runtime
chmod 0755 \
    /usr/sbin/lan-remote \
    /etc/init.d/lan-remote-runtime \
    "$TARGET_DIR/scripts/backup.sh" \
    "$TARGET_DIR/scripts/install-openwrt.sh" \
    "$TARGET_DIR/scripts/set-pc-id.sh" \
    "$TARGET_DIR/scripts/status.sh"
chmod 0755 \
    "$TARGET_DIR" \
    "$TARGET_DIR/cache" \
    "$TARGET_DIR/cache/rustdesk-web" \
    "$TARGET_DIR/data/postgres" \
    "$TARGET_DIR/www" \
    "$TARGET_DIR/www/rdp" \
    "$TARGET_DIR/www/web"
chown 1000:1000 "$TARGET_DIR/data/guacamole-drive"
chmod 0700 "$TARGET_DIR/data/guacamole-drive"

if [ ! -s "$TARGET_DIR/secrets/guacamole-db-password" ]; then
    openssl rand -hex 32 > "$TARGET_DIR/secrets/guacamole-db-password"
fi
chown 0:1001 "$TARGET_DIR/secrets/guacamole-db-password"
chmod 0640 "$TARGET_DIR/secrets/guacamole-db-password"

/etc/init.d/lan-remote-runtime enable
/etc/init.d/lan-remote-runtime restart
wait_for_socket

export DOCKER_HOST=$SOCKET
export DOCKER_CONFIG=$TARGET_DIR/docker-cli

log 'Installing Compose from the verified Docker official CLI image'
"$TARGET_DIR/bin/docker" pull "$COMPOSE_CLI_IMAGE"
"$TARGET_DIR/bin/docker" rm -f lan-remote-compose-extract >/dev/null 2>&1 || true
"$TARGET_DIR/bin/docker" create --name lan-remote-compose-extract "$COMPOSE_CLI_IMAGE" true >/dev/null
"$TARGET_DIR/bin/docker" cp \
    lan-remote-compose-extract:/usr/local/libexec/docker/cli-plugins/docker-compose \
    "$TARGET_DIR/docker-cli/cli-plugins/docker-compose"
"$TARGET_DIR/bin/docker" rm lan-remote-compose-extract >/dev/null
compose_hash=$(sha256sum "$TARGET_DIR/docker-cli/cli-plugins/docker-compose" | awk '{print $1}')
require_value "$compose_hash" "$COMPOSE_SHA256" 'Docker Compose SHA-256'
chmod 0755 "$TARGET_DIR/docker-cli/cli-plugins/docker-compose"
"$TARGET_DIR/bin/docker" compose version | grep -q "v${COMPOSE_VERSION}"

log 'Pulling pinned application versions'
cd "$TARGET_DIR"
"$TARGET_DIR/bin/docker" compose pull

log 'Generating a loopback-only Tomcat connector'
"$TARGET_DIR/bin/docker" run --rm --network none --entrypoint cat guacamole/guacamole:1.6.0 \
    /usr/local/tomcat/conf/server.xml > "$TARGET_DIR/runtime/tomcat-server.xml"
sed -i 's/<Connector port="8080"/<Connector address="127.0.0.1" port="8080"/' \
    "$TARGET_DIR/runtime/tomcat-server.xml"
grep -q '<Connector address="127.0.0.1" port="8080"' "$TARGET_DIR/runtime/tomcat-server.xml"
chmod 0644 "$TARGET_DIR/runtime/tomcat-server.xml"
chmod 0644 "$TARGET_DIR/runtime/rustdesk-web-runtime.sha256"

log 'Verifying the bundled accountless RustDesk Web runtime'
(
    cd "$TARGET_DIR/www/web"
    sha256sum -c "$TARGET_DIR/runtime/rustdesk-web-runtime.sha256"
)

log 'Initializing Guacamole storage and the Windows RDP connection'
"$TARGET_DIR/bin/docker" compose up -d postgres
attempt=0
until "$TARGET_DIR/bin/docker" compose exec -T postgres \
    pg_isready -h 127.0.0.1 -p 55432 -U guacamole -d guacamole_db >/dev/null 2>&1; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 31 ] || { echo 'ERROR: PostgreSQL did not become ready.' >&2; exit 1; }
    sleep 2
done

schema=$(
    "$TARGET_DIR/bin/docker" compose exec -T postgres \
        psql -h 127.0.0.1 -p 55432 -U guacamole -d guacamole_db -tAc \
        "SELECT to_regclass('public.guacamole_entity') IS NOT NULL"
)
if [ "$schema" != t ]; then
    "$TARGET_DIR/bin/docker" run --rm --network none guacamole/guacamole:1.6.0 \
        /opt/guacamole/bin/initdb.sh --postgresql |
        "$TARGET_DIR/bin/docker" compose exec -T postgres \
            psql -h 127.0.0.1 -p 55432 -U guacamole -d guacamole_db -v ON_ERROR_STOP=1
fi
"$TARGET_DIR/bin/docker" compose exec -T postgres \
    psql -h 127.0.0.1 -p 55432 -U guacamole -d guacamole_db -v ON_ERROR_STOP=1 \
    < "$TARGET_DIR/runtime/bootstrap.sql"

log 'Starting Guacamole and RustDesk'
"$TARGET_DIR/bin/docker" compose up -d
attempt=0
while [ ! -s "$TARGET_DIR/data/rustdesk/id_ed25519.pub" ]; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 31 ] || { echo 'ERROR: RustDesk did not generate its public key.' >&2; exit 1; }
    sleep 1
done
rustdesk_key=$(tr -d '\r\n' < "$TARGET_DIR/data/rustdesk/id_ed25519.pub")
sed "s|__RUSTDESK_KEY__|$rustdesk_key|g" \
    "$TARGET_DIR/www/web/index.html.template" > "$TARGET_DIR/www/web/index.html"
rustdesk_id=尚未写入
[ ! -s "$TARGET_DIR/runtime/rustdesk-id" ] || \
    rustdesk_id=$(tr -cd '0-9' < "$TARGET_DIR/runtime/rustdesk-id")
[ -n "$rustdesk_id" ] || rustdesk_id=尚未写入
sed "s|__RUSTDESK_ID__|$rustdesk_id|g" \
    "$TARGET_DIR/www/rdp/index.html.template" > "$TARGET_DIR/www/rdp/index.html"

log 'Configuring the loopback-only Nginx gateway and narrow OpenWrt firewall rules'
cp -a "$SOURCE_DIR/openwrt/nginx.conf" /etc/nginx/nginx.conf
sed "s|__FILE_PROXY_PASS__|proxy_pass http://$PC_ADDRESS:18080;|" \
    "$SOURCE_DIR/openwrt/nginx-site.conf" > /etc/nginx/conf.d/lan-remote-access.conf
chmod 0644 "$TARGET_DIR/www/rdp/index.html" "$TARGET_DIR/www/web/index.html"
chmod 0644 "$TARGET_DIR/runtime/pc-lan-address"
uci -q delete nginx._lan.enabled || true
uci -q delete nginx._redirect2ssl.enabled || true
uci set nginx.global.uci_enable='false'
uci commit nginx
configure_firewall
/etc/init.d/nginx enable
/etc/init.d/nginx restart
sleep 1
pidof nginx >/dev/null
/usr/sbin/nginx -t -c /etc/nginx/nginx.conf

log 'Deployment staged successfully'
printf 'Backup: /root/lan-remote-access-preinstall-%s.tar.gz\n' "$stamp"
printf 'RustDesk server key was written to the local Web runtime.\n'
printf 'PC LAN address: %s\n' "$PC_ADDRESS"
printf 'Next: bind and configure the WebVPN Agent; no Tailnet gateway is required.\n'
