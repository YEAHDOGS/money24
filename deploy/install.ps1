# Money 24 server install for Windows: the one command to run on the machine that will run Money 24.
#
#   winget install --id Git.Git -e          (if git is missing)
#   git clone https://github.com/YEAHDOGS/money24
#   cd money24
#   powershell -ExecutionPolicy Bypass -File deploy\install.ps1 [-From C:\path\money24-move.tar.gz]
#
# Installs what is missing with winget (Git, Node.js LTS, Go, cloudflared), then hands over to
# scripts/server.js install: it pins the release key, builds the newest signed release, registers the
# Money24 logon task and starts it. From then on it keeps itself on the newest signed release.
# -From restores .env and the data packed on the old machine (node scripts/server.js pack there).
# Run it as the Windows user that will be signed in to the AI apps; no administrator rights needed.
param(
  [string]$From = '',
  [switch]$NoStart
)
$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Tools = @(
  @{ Command = 'git'; Id = 'Git.Git' },
  @{ Command = 'node'; Id = 'OpenJS.NodeJS.LTS' },
  @{ Command = 'go'; Id = 'GoLang.Go' }
)
$Cloudflared = 'C:\Program Files (x86)\cloudflared\cloudflared.exe'

function Update-Path {
  $env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' + [Environment]::GetEnvironmentVariable('Path', 'User')
}

if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
  throw 'winget is missing. Install "App Installer" from the Microsoft Store, then run this again.'
}
foreach ($tool in $Tools) {
  if (Get-Command $tool.Command -ErrorAction SilentlyContinue) { continue }
  Write-Host "Installing $($tool.Id)..."
  winget install --id $tool.Id --exact --silent --accept-package-agreements --accept-source-agreements
  Update-Path
}
if (-not (Test-Path $Cloudflared) -and -not (Get-Command cloudflared -ErrorAction SilentlyContinue)) {
  Write-Host 'Installing cloudflared (the tunnel to your phone)...'
  winget install --id Cloudflare.cloudflared --exact --silent --accept-package-agreements --accept-source-agreements
}
Update-Path
if (-not (Test-Path (Join-Path $Root '.git'))) {
  throw "$Root is not a git clone, and updates need one. Run: git clone https://github.com/YEAHDOGS/money24"
}

$arguments = @((Join-Path $Root 'scripts\server.js'), 'install')
if ($From) { $arguments += @('--from', (Resolve-Path $From).Path) }
if ($NoStart) { $arguments += '--no-start' }
& node @arguments
exit $LASTEXITCODE
