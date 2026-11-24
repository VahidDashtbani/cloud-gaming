param(
  [string]$InstallDir = "$env:ProgramFiles\CloudGamingAgent",
  [string]$PairToken = ""
)

New-Item -ItemType Directory -Force -Path "C:\CloudGames" | Out-Null
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

$pairScript = @"
@echo off
cd /d "$InstallDir"
set /p TOK="Token ra vared konid (cg1_...): "
echo %TOK% > "$InstallDir\token.txt"
windows-agent.exe -token "%TOK%" -node %COMPUTERNAME% -max 5
pause
"@

Set-Content -Path "$InstallDir\connect-token.bat" -Value $pairScript -Encoding ASCII

if ($PairToken -ne "") {
  Set-Content -Path "$InstallDir\token.txt" -Value $PairToken -Encoding ASCII
  $autoStart = @"
@echo off
cd /d "$InstallDir"
windows-agent.exe -token "$PairToken" -node %COMPUTERNAME% -max 5
pause
"@
  Set-Content -Path "$InstallDir\start-agent.bat" -Value $autoStart -Encoding ASCII
}

Write-Host "Installed in $InstallDir. Run connect-token.bat to enter your pairing token."
