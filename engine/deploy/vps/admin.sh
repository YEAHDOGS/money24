#!/bin/bash
set -euo pipefail
umask 077

readonly APP=/opt/money24
readonly STATE=/var/lib/money24
readonly DATA=$STATE/data
readonly READY=$STATE/.ready
readonly ENV_FILE=/etc/money24.env
readonly SWAP=/swapfile-money24
readonly UNIT_DIR=/etc/systemd/system
readonly TUNNEL=money24-cloudflared.service
readonly BACKUP=m24backup.service
readonly TIMER=m24backup.timer
readonly SERVICES=(newsd.service m24d.service "$TUNNEL")
readonly CHECKS=(http://127.0.0.1:8024/healthz http://127.0.0.1:8025/healthz http://127.0.0.1:8026/ready)
readonly HEALTH_ATTEMPTS=20
readonly HEALTH_WAIT=2
readonly GUARD=$APP/current/m24guard
readonly GUARD_TIMER=m24guard.timer
readonly GATE=money24-gate.service
readonly WARP_DEADMAN=money24-warp-deadman
readonly WARP_DEADMAN_SECONDS=180
readonly WARP_KEYRING=/usr/share/keyrings/cloudflare-warp-archive-keyring.gpg
# Google's IAP source range must bypass WARP or SSH stops answering. Public IPv6 stays direct;
# only IPv4, which this machine has no address for, goes through WARP.
readonly WARP_BYPASS=(35.235.240.0/20 2000::/3)
readonly NEWS_DB=news/news.db

die() {
    printf '%s\n' "$*" >&2
    exit 1
}

setup() {
    source /etc/os-release
    [[ $ID == debian && $VERSION_ID == 12 && $(uname -m) == x86_64 ]] ||
        die 'The VPS kit needs Debian 12 amd64.'
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl sqlite3 gnupg
    install -d -m 0755 /usr/share/keyrings
    curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg -o /usr/share/keyrings/cloudflare-main.gpg
    chmod 0644 /usr/share/keyrings/cloudflare-main.gpg
    printf '%s\n' 'deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared bookworm main' |
        install -m 0644 /dev/stdin /etc/apt/sources.list.d/cloudflared.list
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y cloudflared

    if ! id money24 >/dev/null 2>&1; then
        useradd --system --user-group --home-dir "$DATA" --shell /usr/sbin/nologin money24
    fi
    install -d -m 0755 "$APP" "$APP/releases" "$STATE"
    install -d -m 0700 -o money24 -g money24 "$DATA" "$DATA/news" "$DATA/trading"
    if ! swapon --noheadings --raw --output NAME | grep -Fxq "$SWAP"; then
        if [[ ! -e $SWAP ]]; then
            fallocate -l 1G "$SWAP"
        fi
        chmod 0600 "$SWAP"
        mkswap "$SWAP"
        swapon "$SWAP"
    fi
    if ! grep -Fq "$SWAP " /etc/fstab; then
        printf '%s none swap sw 0 0\n' "$SWAP" >> /etc/fstab
    fi

    # INSTALL_UNITS

    install -d -m 0755 /etc/systemd/journald.conf.d
    printf '%s\n' '[Journal]' 'SystemMaxUse=64M' 'RuntimeMaxUse=32M' |
        install -m 0644 /dev/stdin /etc/systemd/journald.conf.d/money24.conf
    systemctl restart systemd-journald
    systemctl daemon-reload
    systemctl enable "${SERVICES[@]}" "$TIMER"
    # The guard meters from the first boot; it waits for its binary, which deploy installs.
    systemctl enable --now "$GUARD_TIMER"
}

# The machine has no public IPv4 (Google bills one hourly), so IPv4-only services go through
# Cloudflare WARP's free tunnel, which rides on IPv6. A dead-man timer turns WARP off again
# unless vps.js confirms over a fresh IAP SSH session that the machine is still reachable.
warp() {
    install -d -m 0755 /usr/share/keyrings
    curl -fsSL https://pkg.cloudflareclient.com/pubkey.gpg | gpg --yes --dearmor -o "$WARP_KEYRING"
    chmod 0644 "$WARP_KEYRING"
    printf 'deb [signed-by=%s] https://pkg.cloudflareclient.com/ bookworm main\n' "$WARP_KEYRING" |
        install -m 0644 /dev/stdin /etc/apt/sources.list.d/cloudflare-client.list
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y cloudflare-warp
    warp-cli --accept-tos registration show >/dev/null 2>&1 || warp-cli --accept-tos registration new
    local range
    for range in "${WARP_BYPASS[@]}"; do
        warp-cli --accept-tos tunnel ip add "$range" >/dev/null 2>&1 || true
    done
    warp-cli --accept-tos mode warp
    systemctl stop "$WARP_DEADMAN.timer" 2>/dev/null || true
    systemd-run --unit="$WARP_DEADMAN" --on-active="$WARP_DEADMAN_SECONDS" /usr/bin/warp-cli --accept-tos disconnect
    warp-cli --accept-tos connect
    sleep 5
    warp-cli --accept-tos status
}

# Which services answer, over IPv4 and over IPv6. Without WARP, IPv4 fails everywhere.
netcheck() {
    local hosts=(
        api.elections.kalshi.com api.exchange.coinbase.com api.dexscreener.com pumpportal.fun
        frontend-api-v3.pump.fun api.rugcheck.xyz api.telegram.org query1.finance.yahoo.com
        paper-api.alpaca.markets data.alpaca.markets discord.com gateway.discord.gg
        api.anthropic.com api.openai.com api.x.ai generativelanguage.googleapis.com
        r2.cloudflarestorage.com region1.v2.argotunnel.com deb.debian.org pkg.cloudflare.com
    )
    local host family code
    printf '%-36s %-6s %-6s\n' host IPv4 IPv6
    for host in "${hosts[@]}"; do
        printf '%-36s' "$host"
        for family in -4 -6; do
            code=$(curl "$family" -s -o /dev/null --max-time 8 -w '%{http_code}' "https://$host/" 2>/dev/null) || code=fail
            [[ $code == 000 ]] && code=fail
            printf ' %-6s' "$code"
        done
        printf '\n'
    done
    printf '%-36s' 'Telegram MTProto 149.154.167.50:443'
    if timeout 6 bash -c '</dev/tcp/149.154.167.50/443' 2>/dev/null; then printf ' ok\n'; else printf ' fail\n'; fi
    printf '%s\n' 'Any HTTP code means the host answered; fail means it is unreachable on that family.'
}

# Lists data files for the PC's pull-backup: size, modification time, path. The news database is
# never listed or copied: m24backup writes its finished days into news/days/ (once each) and the
# open day into news/state.jsonl.gz, as the money24 user so the nightly backup can read them, and
# those files are listed instead. If the export fails the rest of the data is still listed.
pull_manifest() {
    local room
    runuser -u money24 -- env GOMEMLIMIT=48MiB "$APP/current/m24backup" --export-news --data "$DATA" >&2 ||
        printf '%s\n' 'News export failed; the PC copy will lack the newest news days.' >&2
    # No guard answer means no proven room: the PC then pulls nothing.
    room=$("$GUARD" status 2>/dev/null | sed -n 's/.*"backup_room_bytes": *\([0-9]*\).*/\1/p' | head -n 1) || room=0
    printf 'ROOM\t%s\n' "${room:-0}"
    find "$DATA" -type f ! -name '*-wal' ! -name '*-shm' ! -name '*-journal' ! -path "$DATA/$NEWS_DB" \
        ! -path '*/.export-*' ! -path '*/.state-*' -printf 'F\t%s\t%T@\t%P\n'
}

# vps.js writes the requested paths here, one per line.
pull_list() {
    # PULL_LIST
    :
}

# Packs the requested files into a private archive the SSH user can download.
pull_pack() {
    local stage rel list
    stage=$(mktemp -d /tmp/money24.XXXXXXXXXX)
    # Removed on any exit, die included, unless the archive is handed over below.
    trap "rm -rf -- '$stage'" EXIT
    list=$stage/files
    mkdir "$stage/tree"
    while IFS= read -r rel; do
        [[ $rel =~ ^[A-Za-z0-9._/-]+$ && $rel != *..* ]] || die "Refusing pull path: $rel"
        [[ -f $DATA/$rel ]] || continue # removed since the manifest
        mkdir -p "$stage/tree/$(dirname -- "$rel")"
        [[ $rel != "$NEWS_DB"* ]] || die "Refusing to copy the live news database: $rel"
        cp -p -- "$DATA/$rel" "$stage/tree/$rel"
        printf '%s\n' "$rel" >> "$list"
    done < <(pull_list)
    [[ -s $list ]] || die 'Nothing to pack.'
    tar -czf "$stage/pull.tar.gz" -C "$stage/tree" -T "$list"
    rm -rf -- "$stage/tree" "$list"
    chown -R "${SUDO_USER:-root}" "$stage"
    printf 'PACK\t%s\t%s\n' "$stage" "$(stat -c %s "$stage/pull.tar.gz")"
    trap - EXIT # pull-clean removes it after the download
}

check_health() {
    local url attempt
    for url in "${CHECKS[@]}"; do
        for ((attempt = 1; attempt <= HEALTH_ATTEMPTS; attempt++)); do
            if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
                break
            fi
            sleep "$HEALTH_WAIT"
        done
        if ((attempt > HEALTH_ATTEMPTS)); then
            printf 'Health check did not pass at %s. Run vps.js logs.\n' "$url" >&2
            return 1
        fi
    done
}

set_release() {
    local link=$1 target=$2
    ln -s -- "$target" "$APP/$link.next"
    mv -Tf -- "$APP/$link.next" "$APP/$link"
}

activate_release() {
    local target=$1 old=$2
    set_release current "$target"
    if [[ -f $READY ]]; then
        systemctl stop "$TIMER" "$BACKUP"
        if ! systemctl restart "${SERVICES[@]}" || ! check_health; then
            set_release current "$old"
            systemctl restart "${SERVICES[@]}"
            systemctl start "$TIMER"
            return 1
        fi
        systemctl start "$TIMER"
    else
        printf '%s\n' 'Release staged. Services stay off until move-data completes.'
    fi
    if [[ -n $old ]]; then
        set_release previous "$old"
    fi
}

deploy() {
    local archive=$1 staging=$2 old dir
    [[ -s $ENV_FILE ]] || die 'Run vps.js env first.'
    dir=$(mktemp -d "$APP/releases/release.XXXXXXXXXX")
    tar -xf "$archive" --no-same-owner --no-same-permissions -C "$dir"
    chmod 0755 "$dir" "$dir/m24d" "$dir/newsd" "$dir/m24backup" "$dir/m24guard"
    chmod -R a+rX "$dir/web"
    test -f "$dir/web/home.html"
    old=$(readlink "$APP/current" || true)
    if ! activate_release "$dir" "$old"; then
        die 'New release failed its checks. The previous release is selected.'
    fi
    # Keep current and previous only. This root-owned directory holds release builds.
    for dir in "$APP"/releases/release.*; do
        [[ $dir == "$(readlink "$APP/current")" || $dir == "$(readlink "$APP/previous" || true)" ]] ||
            rm -rf -- "$dir"
    done
    rm -f -- "$staging/payload.tar"
}

move_data() {
    local archive=$1 force=$2 stage file rel old_data ready=no
    [[ -x $APP/current/m24d && -s $ENV_FILE ]] || die 'Run vps.js env and deploy first.'
    stage=$(mktemp -d "$STATE/.incoming.XXXXXXXXXX")
    trap "rm -rf -- '$stage'" EXIT
    tar -xf "$archive" --no-same-owner --no-same-permissions -C "$stage"
    mkdir -p "$stage/news" "$stage/trading"
    chown -R money24:money24 "$stage"
    chmod -R u+rwX,go-rwx "$stage"
    if [[ -f $READY ]]; then
        ready=yes
        [[ $force == force ]] || die 'The VPS has already been cut over. Replacing its data requires --force.'
        systemctl stop "$TIMER" "$BACKUP" "${SERVICES[@]}"
    fi
    if [[ $force != force ]]; then
        # Existing server-only files are also protected. Tar implementations can round timestamps.
        while IFS= read -r -d '' file; do
            rel=${file#"$DATA/"}
            if [[ ! -f $stage/$rel ]] || { [[ $file -nt $stage/$rel ]] && ! cmp -s "$file" "$stage/$rel"; }; then
                die 'VPS has newer or server-only data. Review it before using --force.'
            fi
        done < <(find "$DATA" -type f -print0)
    fi
    old_data=$(mktemp -d "$STATE/.previous-data.XXXXXXXXXX")
    rmdir "$old_data"
    mv -T -- "$DATA" "$old_data"
    if ! mv -T -- "$stage" "$DATA"; then
        mv -T -- "$old_data" "$DATA"
        if [[ $ready == yes ]]; then
            systemctl start "${SERVICES[@]}" "$TIMER"
        fi
        die 'Data activation failed. The previous directory is back in place.'
    fi
    touch "$READY"
    if ! systemctl start "${SERVICES[@]}" || ! check_health; then
        # Preserve both trees and keep the VPS off until the operator fixes the issue.
        rm -f -- "$READY"
        systemctl stop "${SERVICES[@]}"
        printf 'Imported data is preserved. Previous VPS data is at %s.\n' "$old_data" >&2
        die 'Cutover did not pass health checks. Run vps.js logs.'
    fi
    systemctl start "$TIMER"
    if ! rmdir "$old_data/news" "$old_data/trading" "$old_data" 2>/dev/null; then
        printf 'Previous VPS data kept at %s\n' "$old_data"
    fi
    printf '%s\n' 'Data imported and VPS services are healthy.'
}

case "${1:-}" in
    status)
        systemctl show "$GATE" "${SERVICES[@]}" "$BACKUP" "$TIMER" "$GUARD_TIMER" --no-pager \
            -p Id -p ActiveState -p SubState -p NRestarts -p MemoryCurrent -p Result -p NextElapseUSecRealtime
        free -h
        df -h "$STATE"
        du -sh "$DATA" "$APP/releases"
        journalctl --disk-usage
        printf '\n%s\n' 'Egress guard (bytes sent this month on the network card; Money 24 stops at the limit):'
        "$GUARD" status || printf '%s\n' 'Guard binary not installed yet. Run vps.js deploy.'
        warp-cli --accept-tos status 2>/dev/null || true
        exit
        ;;
    logs)
        journalctl --no-pager -n 100 -u newsd -u m24d -u "$TUNNEL" -u "$BACKUP" -u "$TIMER" -u m24guard -u "$GATE"
        exit
        ;;
    netcheck) netcheck; exit ;;
    pull-manifest) pull_manifest; exit ;;
    pull-pack) pull_pack; exit ;;
    pull-clean)
        [[ ${2:-} =~ ^/tmp/money24\.[A-Za-z0-9]{10}$ ]] || die 'Missing pull staging directory.'
        rm -rf -- "$2"
        exit
        ;;
esac

exec 9>/run/lock/money24-vps.lock
flock -x 9
case "${1:-}" in
    setup) setup ;;
    deploy|move-data)
        staging=${2:-}
        [[ $staging =~ ^/tmp/money24\.[A-Za-z0-9]{10}$ && -f $staging/payload.tar ]] ||
            die 'Missing private upload.'
        if [[ $1 == deploy ]]; then
            deploy "$staging/payload.tar" "$staging"
        else
            move_data "$staging/payload.tar" "${3:-}"
        fi
        ;;
    rollback)
        previous=$(readlink "$APP/previous") || die 'No previous release to roll back to.'
        activate_release "$previous" "$(readlink "$APP/current")"
        ;;
    env)
        # INSTALL_ENV
        ;;
    warp) warp ;;
    warp-confirm)
        systemctl stop "$WARP_DEADMAN.timer" 2>/dev/null || true
        printf '%s\n' 'WARP stays on: SSH still answers through IAP.'
        ;;
    *) die 'Unknown VPS operation.' ;;
esac
