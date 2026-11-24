param(
  [string]$InstallDir = "$env:ProgramFiles\CloudGamingAgent",
  [string]$RelayIP = "127.0.0.1"
)

New-Item -ItemType Directory -Force -Path "C:\CloudGames" | Out-Null
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

$startScript = @"
@echo off
cd /d "$InstallDir"
windows-agent.exe -server ws://$RelayIP:8080/ws/node -node %COMPUTERNAME% -max 5
pause
"@

Set-Content -Path "$InstallDir\start-agent.bat" -Value $startScript -Encoding UTF8
Write-Host "Windows Client installed in $InstallDir and games folder ready at C:\CloudGames"
