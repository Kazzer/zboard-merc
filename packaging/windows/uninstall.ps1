<#
Removes the ZEngine logon task and the binaries; keeps %APPDATA%\zboard unless -Purge.
#>
param([switch]$Purge)
$ErrorActionPreference = 'Continue'
$bin = Join-Path $env:LOCALAPPDATA 'zboard\bin'
$cfg = Join-Path $env:APPDATA 'zboard'
Get-ScheduledTask -TaskName 'ZEngine' -ErrorAction SilentlyContinue | Unregister-ScheduledTask -Confirm:$false
Get-Process -Name 'zengine', 'zengine-tray' -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 500
Remove-Item -Recurse -Force (Join-Path $env:LOCALAPPDATA 'zboard') -ErrorAction SilentlyContinue
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
[Environment]::SetEnvironmentVariable('Path', (($userPath -split ';' | Where-Object { $_ -and $_ -ne $bin }) -join ';'), 'User')
if ($Purge) { Remove-Item -Recurse -Force $cfg -ErrorAction SilentlyContinue }
Write-Host "zboard removed$(if (-not $Purge) { "; configuration kept in $cfg" })"
