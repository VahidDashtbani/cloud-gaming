[Setup]
AppName=Cloud Gaming Windows Client
AppVersion=1.0.0
DefaultDirName={autopf}\CloudGamingAgent
DefaultGroupName=Cloud Gaming Agent
OutputDir=..\..\bin
OutputBaseFilename=CloudGamingAgent-Setup
Compression=lzma
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible

[Files]
Source: "..\..\bin\windows-agent.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "install-windows.ps1"; DestDir: "{app}"; Flags: ignoreversion

[Run]
Filename: "powershell.exe"; Parameters: "-ExecutionPolicy Bypass -File ""{app}\install-windows.ps1"" -InstallDir ""{app}"""; Flags: runhidden waituntilterminated
