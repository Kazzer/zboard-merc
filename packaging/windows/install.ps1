<#
Installs ZEngine agent for the current user: the binaries into %LOCALAPPDATA%\zboard\bin (added to the user's PATH),
the example configuration into %APPDATA%\zboard when there is none yet (an existing one is left exactly as it is), and
a logon task "ZEngine" that starts the tray agent and keeps it running.

  .\install.ps1  non-elevated agent (the default; elevated windows keep raw keys)
    -Elevated    the task runs with highest privileges, without a UAC prompt at logon; requires administrator PowerShell
    -NoTask      files only, no logon task
    -Examples    copies the example config.yaml and Mods again, overwriting the files of the same names
#>
param([switch]$Elevated, [switch]$NoTask, [switch]$Examples)
$ErrorActionPreference = 'Stop'
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$bin = Join-Path $env:LOCALAPPDATA 'zboard\bin'
$cfg = Join-Path $env:APPDATA 'zboard'
New-Item -ItemType Directory -Force -Path $bin, (Join-Path $cfg 'mods') | Out-Null

Get-ScheduledTask -TaskName 'ZEngine' -ErrorAction SilentlyContinue | Stop-ScheduledTask -ErrorAction SilentlyContinue
Get-Process -Name 'zengine', 'zengine-tray' -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 500

foreach ($f in 'zengine.exe', 'zengine-tray.exe', 'zboard.exe', 'zprobe.exe') {
    Copy-Item (Join-Path $here $f) $bin -Force
}
if ($Examples -or -not (Test-Path (Join-Path $cfg 'config.yaml'))) {
    Copy-Item (Join-Path $here 'config.yaml') $cfg -Force
    Copy-Item (Join-Path $here 'mods\*.yaml') (Join-Path $cfg 'mods') -Force
}
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $bin) {
    [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ';' + $bin), 'User')
}

$level = 'none'
if (-not $NoTask) {
    $level = if ($Elevated) { 'Highest' } else { 'Limited' }
    $action = New-ScheduledTaskAction -Execute (Join-Path $bin 'zengine-tray.exe') -Argument ('-log "' + (Join-Path $cfg 'zengine.log') + '"')
    $trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit (New-TimeSpan -Seconds 0) -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
    $principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel $level
    Register-ScheduledTask -TaskName 'ZEngine' -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Force | Out-Null
    Start-ScheduledTask -TaskName 'ZEngine'
}
Write-Host "zboard: binaries in $bin (on your PATH from the next shell), configuration in $cfg, logon task: $level"
Write-Host "open the UI with: zboard ui    log: $cfg\zengine.log"
