# Money 24 on Google's free machine

The runner (`m24d`, `newsd`, the Cloudflare tunnel) moves from the PC to one free Google Cloud
`e2-micro`. The kit is built so that it cannot bill by accident: hard cut-offs, not alerts.
Everything runs through `node scripts/vps.js <command>`; add `--dry-run` to any command to print
exactly what it would do without touching anything.

## What is free, and what is not

From Google's free tier page (Compute Engine row, checked 2026-11-21):

> 1 non-preemptible e2-micro VM instance per month in one of the following US regions:
> Oregon: us-west1. Iowa: us-central1. South Carolina: us-east1.
> 30 GB-months standard persistent disk.
> 1 GB of outbound data transfer from North America to all region destinations (excluding China and Australia) per month.

The row says nothing about IP addresses. Google's network pricing page does:

> Static and ephemeral IP addresses in use on standard VM instances: $0.005 / 1 hour
>
> Both static and ephemeral IP addresses assigned to standard VM instances are offered with a
> free tier. This free usage is limited to one hour per month per account.

So a public IPv4, even an ephemeral one, costs about $3.65 a month. The same page:

> You are not charged for external IPv6 address ranges that are assigned to subnets or for
> external IPv6 addresses that are assigned to VM instances.

**The machine therefore has no public IPv4.** It gets an internal IPv4 (for SSH through Google's
Identity-Aware Proxy and the metadata server) and a free public IPv6. Many services answer on IPv6
(Coinbase, DEX Screener, pump.fun, Telegram, Yahoo, Cloudflare's tunnel and R2, Anthropic, OpenAI,
xAI, Gemini, Debian). Some do not: Kalshi, Alpaca, PumpPortal, RugCheck, Discord, GitHub and several
news feeds have no IPv6 address. For those, `vps.js warp` installs Cloudflare WARP (free) and sends
IPv4 through it over IPv6; public IPv6 and Google's SSH range bypass it. `vps.js netcheck` shows
which hosts answer on each family.

## The layers

| Layer | Protects against | Honest limit | Worst case |
| --- | --- | --- | --- |
| **Billing kill switch** (`killswitch`): a $1 budget on the billing account notifies Pub/Sub; a function removes this project's billing account when cost reaches it. | Anything at all that bills: a wrong machine, an IP, egress, a forgotten disk. The budget watches the whole billing account, so keep no other projects on it: the switch can only unlink this one. | Google: "There's a delay between incurring costs and receiving budget notifications." Notifications come "multiple times per day", the first "may take several hours". Disabling billing stops **everything**, free services included, and "Resources might be irretrievably deleted." | Cost that accrues before the notification lands, likely cents past $1. The switch's own parts are inside their free tiers (below). |
| **On-machine guard** (`m24guard`, every 5 minutes): meters bytes sent on the network card this month, persisted across reboots; stops the services at 800 MB. Refuses to start them unless the metadata server and Compute API show a non-preemptible `e2-micro` in a free region, no external IPv4, one `pd-standard` disk of at most 30 GB. Two sightings of a billable machine power it off. If it cannot count for 30 minutes (a broken counter or ledger), it stops the services rather than run blind. | Egress past the free 1 GB, which the kill switch would only see hours later. A machine changed by hand in the console. | It counts all bytes on the card, including traffic Google does not bill (conservative). Up to five minutes of traffic can be lost if the machine crashes rather than shuts down. It cannot remove an IP or a disk; powering off stops compute and releases an ephemeral IPv4, but a disk bills while it exists. | A few MB past 800 MB, still under 1 GB. |
| **Allowlist** (`scripts/vps/freetier.js`): every gcloud command the kit runs must be on a fixed list. Snapshots, static IPs, load balancers, routers (NAT), disks and images may only be listed. An instance create must be STANDARD `e2-micro`, free zone, 30 GB `pd-standard`, no IPv4. `setup` also refuses while the project holds any snapshot, IP, load balancer, router, custom image, or other disk or instance. | The kit itself creating something billable. | It cannot stop you from clicking in the console; the guard and kill switch cover that. | Nothing billable is ever created by the kit. |
| **Backup caps** (`m24backup`): R2 keeps at most 8 GB (free: 10 GB); a night makes at most 4 uploads and 5 list pages (~300 write operations a month of R2's free million) and sends at most 15 MB, never pushing the month past 75% of the guard's limit. `pull-backup` sends at most 50 MB a run, inside the same room. | R2, which has no spending cap of its own; backups eating the egress allowance. | A state archive larger than a night's allowance fails loudly (`vps.js status`, `logs`) rather than sending. | R2 storage stays under its free 10 GB. |

What the kill switch creates, and what each could ever cost (Google's free tiers, checked 2026-11-21):

- **Budget and Cloud Billing API**: no charge.
- **Pub/Sub topic**: free tier 10 GiB of messages a month; budget notifications are a few MB.
- **Cloud Run function** (2nd gen, billed as Cloud Run; 128 MiB, max one instance): free tier 2 million
  requests, 360,000 GB-seconds of memory and 180,000 vCPU-seconds a month; it runs a few thousand
  times a month for about a second.
- **Eventarc trigger**: "Events from Pub/Sub sources: $0 per million events published."
- **Cloud Build** (one build per `killswitch` run, about a minute): free tier 2,500 build-minutes a month.
- **Artifact Registry** (the function's image): free tier 0.5 GB; a cleanup policy keeps two images.
  Past 0.5 GB Google charges $0.000136986 per GiB-hour, about $0.10 per GiB-month.
- **Cloud Logging**: first 50 GiB a month free; the function logs a few lines.

None of these can reach $1 unless the kit is redeployed hundreds of times a month. If one did, the
kill switch would cut billing, including itself.

Optional extra: Google's console offers **spend cap budgets** that pause Cloud Run and Cloud Run
functions automatically. They do not cover Compute Engine ("Spend caps don't pause any on-going,
fixed usage associated with persistent resources"), so they add to the kill switch, not replace it.

## Before you start

- A Google account with a Cloud Billing account (Google requires a card even for the free tier).
  If the account is on the Free Trial, the budget counts cost **before** promotional credits, so
  the switch still fires at $1 of real usage.
- A Google Cloud project linked to that billing account. Do **not** lock the project to its billing
  account: Google says "You can't disable billing on a project that's locked to a billing account."
- Google Cloud CLI installed, then `gcloud auth login`.
- Add to `.env` (never commit it):

  ```
  VPS_PROJECT=your-project-id
  VPS_ZONE=us-west1-b
  GCP_BILLING_ACCOUNT=XXXXXX-XXXXXX-XXXXXX
  GCP_BUDGET_USD=1
  ```

  Optional: `VPS_INSTANCE` (default `money24`), `VPS_MONTH_TX_BYTES` (guard limit, default 800000000,
  at most 950000000), `VPS_PULL_DIR` (where `pull-backup` keeps the copy).

## Steps, in order

Run each with `--dry-run` first and read what it prints.

1. **Kill switch first, before anything runs on a machine.**
   `node scripts/vps.js killswitch`. It is safe to run again; if a step fails just after it created a
   service account, Google may still be propagating it: wait a minute and rerun.
2. **Prove it.** `node scripts/vps.js killswitch test` publishes a fake $2 budget message marked as a
   dry run. It passes only when the function logs that it would remove billing and holds the
   permission to. Nothing is disabled. `setup` refuses to run until this has passed for this
   project and budget.
3. **Cloudflare.** Create the tunnel and put its token in `.env` as `M24D_TUNNEL_TOKEN`. For R2
   backups, also create a private bucket and an R2 API token, and set `R2_ACCOUNT_ID`,
   `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET`. Leave all four out to back up with
   `pull-backup` instead.
4. `node scripts/vps.js build` builds the Linux binaries (including `m24guard`) and the desk pages.
5. `node scripts/vps.js setup` audits the project, creates the free network, the machine's read-only
   service account and the machine, checks it is the free one, then installs packages, swap, users
   and the systemd units, including the guard timer.
6. `node scripts/vps.js warp` turns on WARP for IPv4. If SSH stops answering, wait three minutes: a
   dead-man timer turns WARP back off. Then `node scripts/vps.js netcheck`.
7. `node scripts/vps.js env` installs the server settings (values never printed).
8. `node scripts/vps.js deploy` uploads the release; services stay off until the data arrives.
9. **Stop the PC runner.** `node scripts/forever.js --uninstall` (no longer starts at logon) **and then**
   `node scripts/forever.js --stop` (uninstall alone leaves the running services running).
10. `node scripts/vps.js move-data` copies the PC's data and starts the services behind the guard.
11. `node scripts/vps.js status` shows the services, the guard's month total and WARP.

### Backups

- **R2** (if configured): nightly at about 06:30 UTC, within the caps above.
- **PC copy** (free, no Cloudflare account needed): `node scripts/vps.js pull-backup` copies new and
  changed files to `%APPDATA%\Money24-cloud-backup` (or `VPS_PULL_DIR`). To run it every night the
  PC is on, catching up after it was off, run once in PowerShell from the repo folder:

  ```powershell
  $action = New-ScheduledTaskAction -Execute node -Argument 'scripts\vps.js pull-backup' -WorkingDirectory (Get-Location)
  $trigger = New-ScheduledTaskTrigger -Daily -At 7am
  $settings = New-ScheduledTaskSettingsSet -StartWhenAvailable
  Register-ScheduledTask -TaskName 'Money24 cloud pull' -Action $action -Trigger $trigger -Settings $settings
  ```

Both backups send bytes out of Google, so both count toward the guard's monthly total. If the kill
switch ever fires and Google deletes the machine, restore from either copy: `m24backup --restore
--data <new folder> --guard-state ""` with the R2 settings in the environment, or the PC copy as-is.

## If something trips

- **Guard stopped the services** (`vps.js logs` says `stopping Money 24`): the month's egress hit the
  limit. They resume by themselves when the billing month turns (Pacific time). Look at what sent
  the bytes before raising the limit. `cannot meter egress` means the counter or the ledger at
  `/var/lib/m24guard/state.json` is broken; the services come back once counting works again.
  Backups also send nothing while the ledger is more than 30 minutes old.
- **Guard powered the machine off**: someone changed the machine type, zone, disk or IP. Undo it in
  the console, then start the machine.
- **Kill switch fired**: the project has no billing account and everything in it is stopped. Find
  the cost in the Billing console first. Re-linking billing is manual and Google says "there's no
  guarantee of service recovery".
