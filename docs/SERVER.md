# Money 24 as a server

One machine runs Money 24 for good: the news service, the trading runner, the pages and the tunnel.
Today that is a spare Windows desktop at home; later it can be a Linux box (Amazon EC2). Everything
else (this PC's browser or app, the phone) is a window into it, through the Cloudflare tunnel.

```
phone, PC  →  money24.dogs.red/desk  →  Cloudflare Access (email PIN)  →  the Worker  →  the tunnel
           →  the server: newsd (news), m24d (runner, plays, AI asks), the AI apps signed in there
```

It installs with one command, starts at boot, restarts anything that stops, and keeps itself on the
newest **signed release** with no one touching it. A release that will not run is rolled back on its own.

## Moving to the spare desktop

On **this PC** (the one running Money 24 now):

```powershell
npm run forever -- --stop        # stop it, so the data is still while it is packed
npm run forever -- --uninstall   # and never start it here again: two runners must not share the tunnel
npm run server -- pack           # writes money24-move.tar.gz: .env, the data, and files .env points to
```

`money24-move.tar.gz` holds your secrets (the tunnel token, broker keys, the Telegram session). Carry
it on a USB stick or over SSH, and delete it once it is restored.

On the **spare desktop**, signed in as the Windows user that will run Money 24:

```powershell
winget install --id Git.Git -e
git clone https://github.com/YEAHDOGS/money24
cd money24
powershell -ExecutionPolicy Bypass -File deploy\install.ps1 -From D:\money24-move.tar.gz
```

`install.ps1` installs what is missing with winget (Git, Node.js LTS, Go, cloudflared), restores the
packed file, then runs `node scripts/server.js install`:

1. **Pins the release key** in `%AppData%\Money24\release-signers` and prints its fingerprint. Compare
   it with this PC's: `npm run release -- --key`.
2. **Builds the newest signed release**. The first build takes a few minutes; later ones take seconds.
3. **Registers the `Money24` logon task**, which runs the launcher with no window, and sets the PC to
   never sleep on mains power.
4. **Starts it.** The desk is at http://127.0.0.1:8025/desk/ there, and at money24.dogs.red/desk from
   anywhere once the tunnel is up.

Two settings on the spare desktop finish the job:

- **Sign in automatically** after a reboot (a power cut, a Windows update). The task starts at logon, and
  the tray icon shows on that desktop. Sysinternals Autologon stores the password encrypted.
- **Sign in to the AI apps** there, once each (docs/AI.md): `claude`, `codex login`, Antigravity,
  and `npm install -g opencode-ai`. Then `cd engine && go run ./cmd/m24d --ai-check`.

Then **SSH**, for when you want a terminal on it: Settings → System → Optional features → OpenSSH
Server, then `Start-Service sshd; Set-Service sshd -StartupType Automatic`. Key logins only.

## Linux (and EC2 later)

```sh
git clone https://github.com/YEAHDOGS/money24 && cd money24
sh deploy/install.sh --from ~/money24-move.tar.gz
```

`install.sh` needs git, curl, tar and ssh-keygen. It installs Node.js LTS and Go into
`~/.local/share/money24-tools` when they are missing or too old, from nodejs.org and go.dev over HTTPS,
each archive checked against its published SHA-256. It needs no root. It writes the systemd user unit
`money24.service` and turns on lingering, so Money 24 starts at boot with nobody logged in. x86-64 and
ARM (Graviton) both work.

On EC2: a `t4g.medium` (4 GB: the services and the AI apps together need a few) on Ubuntu 24.04. No
public port is needed, since the tunnel dials out.
Open only SSH in the security group, from your own address. Install cloudflared from Cloudflare's
package repository. The AI apps sign in without a browser: `claude setup-token`,
`codex login --device-auth`, and OpenCode's free models need no account.

Day to day: `systemctl --user status money24`, `journalctl --user -u money24 -f`, and the logs in `.run/logs/`.

## Updates

**A push to master does not deploy.** A release does, and only the owner's PC can make one:

```sh
npm run release       # signs a release tag for master as it is on GitHub, and pushes the tag
```

Every server checks GitHub every 5 minutes (`M24_UPDATE_MINUTES` in `.env`; `off` checks only when
asked). When it finds a newer release:

1. **It checks the signature** against the key it pinned at install. An unsigned tag, a tag signed by any
   other key, an old release pushed again under a new name, or anything older than what runs is ignored
   and logged.
2. **It builds it beside the running release**, in `.run/stage`: its exact packages (`npm ci` checks every
   one against `package-lock.json`, and runs no package scripts), both services and the pages. The
   services keep running meanwhile. A release that does not build changes nothing.
3. **It switches**: the services stop, the new build takes the old one's place (the old one is kept in
   `.run/prev`), the checkout moves to the release, and the launcher starts the new supervisor. The desk
   is down for about a second.
4. **Probation.** For three minutes newsd and m24d must keep answering their health checks with no
   restart. If one crashes twice, or fails probation, it **rolls back on its own**: the previous build and
   checkout come back, and that release is never tried again. A supervisor that cannot even start is
   rolled back by the launcher.

| Command (on the server) | What it does |
|---|---|
| `node scripts/server.js status` | the release, the one it rolls back to, the last rollback, every service |
| `node scripts/server.js update` | check for a signed release now |
| `node scripts/server.js rollback` | go back to the release before |
| `node scripts/forever.js --stop` | stop everything (the logon task, or systemd, starts it again at boot) |

Over SSH those are all there is to it, and nothing needs SSH: a release reaches the server on its own.

### Why it cannot be spoofed

- The server trusts **one key, pinned outside the repo**. Nothing it fetches can change that key: a new
  key in a later checkout is reported and ignored. Changing it takes a person on the server: delete
  `release-signers` and install again.
- The private key lives **only on the owner's PC** (`~/.ssh/money24_release`). It is not on GitHub and not
  in CI. Someone who hijacked DNS, the network or the GitHub account could serve any code they like,
  and the server would not run it, because they cannot sign it.
- The signature covers the commit, and through it every file, `package-lock.json` and `go.sum`, so the
  packages are pinned by hash too.
- A release is never older than the one running (no downgrade to an old bug), and the name inside the
  signed tag must match the tag.

If the release key is lost, make a new one (`npm run release -- --setup`, commit
`deploy/release-signers`), then on each server delete `release-signers` and run install again.

## The AI apps live on the server

Money 24 asks Claude, ChatGPT, Gemini and OpenCode through their command-line apps, signed in with your
own plans (docs/AI.md). Those apps run where the runner runs, so they are **signed in on the server**,
not on the phone. The phone, and this PC, use them through the desk: **Ask Claude** on a play from the
phone runs Claude Code on the server and shows the answer on the phone.

It has to be the server: a phone cannot run those apps, and paying for API keys instead is off the
table. The server also has to answer a news story in seconds while the phone is in your pocket.

## What runs where

| Where | What |
|---|---|
| The server | `server.js run` (launcher) → `forever.js` (supervisor) → newsd, m24d, cloudflared; the AI apps; the data in `%AppData%\Money24` or `~/.config/Money24` |
| `.run/` | the running build, `prev/` (rollback), `stage/` (the next release), `release.json`, `logs/` |
| Cloudflare | Access, the Worker and the tunnel: they carry the desk to the phone and store nothing |
| This PC | the release key, the code, and a browser or the app pointed at the desk |
