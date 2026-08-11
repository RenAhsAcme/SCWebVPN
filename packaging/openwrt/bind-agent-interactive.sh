#!/bin/sh
set -eu

agent=/usr/libexec/scwebvpn/current/webvpn-agent
identity=/etc/scwebvpn/identity
key=$identity/agent.key
agent_id_file=$identity/agent.id
controller=${WEBVPN_CONTROLLER_URL:-${1:-}}

case "$controller" in
    https://*/*|https://*) ;;
    *)
        echo "usage: $0 https://vpn.example.com" >&2
        exit 2
        ;;
esac
controller=${controller%/}

[ -t 0 ] && [ -t 1 ] || {
    echo "binding requires an interactive terminal" >&2
    exit 1
}
[ -x "$agent" ]
[ -d "$identity" ]
[ ! -e "$agent_id_file" ] || {
    echo "Agent is already bound" >&2
    exit 1
}

printf 'Paste the one-time binding code: '
trap 'unset code' EXIT HUP INT TERM
if ! IFS= read -r -s code; then
    printf '\n' >&2
    exit 1
fi
printf '\n'

[ -n "$code" ] || {
    echo "binding code is empty" >&2
    exit 1
}
agent_id=$(
    printf '%s\n' "$code" |
        start-stop-daemon -S -c webvpn:webvpn -x "$agent" -- \
            bind -controller "$controller" -key "$key" -name OpenWrt
)
unset code
trap - EXIT HUP INT TERM

case "$agent_id" in
    ''|*[!A-Za-z0-9_-]*)
        echo "Agent returned an invalid identifier" >&2
        exit 1
        ;;
esac
[ "${#agent_id}" -eq 22 ] || {
    echo "Agent returned an invalid identifier" >&2
    exit 1
}

umask 077
printf '%s\n' "$agent_id" >"$agent_id_file"
chown webvpn:webvpn "$agent_id_file"
printf 'Agent bound: %s\n' "$agent_id"
