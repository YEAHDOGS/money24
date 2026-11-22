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

die() {
    printf '%s\n' "$*" >&2
    exit 1
}

setup() {
    source /etc/os-release
    [[ $ID == debian && $VERSION_ID == 12 && $(uname -m) == x86_64 ]] ||
        die 'The VPS kit needs Debian 12 amd64.'
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl sqlite3 vnstat
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
    systemctl enable --now vnstat.service
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
    chmod 0755 "$dir" "$dir/m24d" "$dir/newsd" "$dir/m24backup"
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
        systemctl show "${SERVICES[@]}" "$BACKUP" "$TIMER" --no-pager \
            -p Id -p ActiveState -p SubState -p NRestarts -p MemoryCurrent -p Result -p NextElapseUSecRealtime
        free -h
        df -h "$STATE"
        du -sh "$DATA" "$APP/releases"
        journalctl --disk-usage
        printf '\n%s\n' 'Monthly interface traffic (TX includes all outbound traffic, use Cloud Billing for billable egress):'
        vnstat --months --limit 1 || true
        exit
        ;;
    logs)
        journalctl --no-pager -n 100 -u newsd -u m24d -u "$TUNNEL" -u "$BACKUP" -u "$TIMER"
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
    *) die 'Unknown VPS operation.' ;;
esac
