# Money 24's icon in the Windows system tray, shown for as long as the supervisor (scripts/forever.js)
# runs the services. Nothing of Money 24 runs out of sight: the icon is the proof it is running, and its
# menu opens the desk, shows each service's state, opens the logs, restarts or stops everything.
# Started by forever.js with Windows PowerShell (built in, no install); it exits on its own when the
# supervisor is gone. Paper trading only; nothing here can place a real order.
param(
  [Parameter(Mandatory = $true)][int]$Supervisor,
  [Parameter(Mandatory = $true)][string]$Root,
  [Parameter(Mandatory = $true)][string]$Desk,
  [Parameter(Mandatory = $true)][string]$Health,
  [Parameter(Mandatory = $true)][string]$Logs,
  [string]$Icon = '',
  [string]$Task = 'Money24'
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$CHECK_EVERY_MS = 5000
$HEALTH_TIMEOUT_MS = 2000
$TIP_RUNNING = 'Money 24: running (paper)'
$TIP_STARTING = 'Money 24: starting...'
$TIP_DOWN = 'Money 24: runner not answering'
$TITLE = 'Money 24'
$WELCOME = 'Money 24 is running. Right-click this icon to open the desk, see status, or stop it.'
$FOREVER = Join-Path $Root 'scripts\forever.js'
$NODE = (Get-Command node -ErrorAction SilentlyContinue).Source

$tray = New-Object System.Windows.Forms.NotifyIcon
$tray.Icon = if ($Icon -and (Test-Path $Icon)) { New-Object System.Drawing.Icon $Icon } else { [System.Drawing.SystemIcons]::Application }
$tray.Text = $TIP_STARTING
$tray.Visible = $true

# Runs forever.js with a flag in a hidden, separate process, so stopping the supervisor never stops the
# command that asked for it.
function Start-Forever([string]$Arguments) {
  if (-not $NODE) { return }
  Start-Process -FilePath $NODE -ArgumentList "`"$FOREVER`" $Arguments" -WorkingDirectory $Root -WindowStyle Hidden
}

function Show-Status {
  if (-not $NODE) { return }
  $text = & $NODE $FOREVER --status 2>&1 | Out-String
  [System.Windows.Forms.MessageBox]::Show($text.Trim(), "$TITLE status", 'OK', 'Information') | Out-Null
}

function Restart-Money24 {
  $installed = $false
  try { schtasks /Query /TN $Task 2>$null | Out-Null; $installed = ($LASTEXITCODE -eq 0) } catch { }
  $start = if ($installed) { "schtasks /Run /TN $Task" } else { "`"$NODE`" `"$FOREVER`" --no-build" }
  $command = "`"$NODE`" `"$FOREVER`" --stop & $start"
  Start-Process -FilePath 'cmd.exe' -ArgumentList "/c $command" -WorkingDirectory $Root -WindowStyle Hidden
}

$menu = New-Object System.Windows.Forms.ContextMenuStrip
$open = $menu.Items.Add('Open the desk')
$open.Font = New-Object System.Drawing.Font($open.Font, [System.Drawing.FontStyle]::Bold)
$open.add_Click({ Start-Process $Desk })
$menu.Items.Add('Status of every service').add_Click({ Show-Status })
$menu.Items.Add('Open the logs folder').add_Click({ Start-Process explorer.exe $Logs })
$menu.Items.Add('-') | Out-Null
$menu.Items.Add('Restart Money 24').add_Click({ Restart-Money24 })
$menu.Items.Add('Stop Money 24').add_Click({
  $answer = [System.Windows.Forms.MessageBox]::Show('Stop every Money 24 service? Recording and paper trading pause until it is started again.', $TITLE, 'YesNo', 'Question')
  if ($answer -eq 'Yes') { Start-Forever '--stop' }
})
$tray.ContextMenuStrip = $menu
$tray.add_DoubleClick({ Start-Process $Desk })
$tray.ShowBalloonTip(5000, $TITLE, $WELCOME, [System.Windows.Forms.ToolTipIcon]::Info)

# Every few seconds: gone supervisor means gone icon; otherwise the tooltip says whether the runner answers.
$timer = New-Object System.Windows.Forms.Timer
$timer.Interval = $CHECK_EVERY_MS
$timer.add_Tick({
  if (-not (Get-Process -Id $Supervisor -ErrorAction SilentlyContinue)) {
    $timer.Stop()
    $tray.Visible = $false
    $tray.Dispose()
    [System.Windows.Forms.Application]::Exit()
    return
  }
  try {
    $request = [System.Net.WebRequest]::Create($Health)
    $request.Timeout = $HEALTH_TIMEOUT_MS
    $response = $request.GetResponse()
    $response.Close()
    $tray.Text = $TIP_RUNNING
  } catch {
    $tray.Text = $TIP_DOWN
  }
})
$timer.Start()

[System.Windows.Forms.Application]::Run()
