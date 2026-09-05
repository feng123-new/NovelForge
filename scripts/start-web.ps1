param([switch]$NoAutopilot)
$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Binary = Join-Path $Root 'novelforge.exe'
if (-not (Test-Path -LiteralPath $Binary -PathType Leaf)) { throw 'Extract the matching Windows release archive first.' }
$Workspace = if ($env:NOVELFORGE_WORKSPACE) { $env:NOVELFORGE_WORKSPACE } else { Join-Path $Root 'workspace' }
$Port = if ($env:NOVELFORGE_PORT) { $env:NOVELFORGE_PORT } else { '48090' }
$Arguments = @('server', '--host', '127.0.0.1', '--port', $Port, '--workspace', $Workspace)
if ($env:NOVELFORGE_CONFIG) { $Arguments += @('--config', $env:NOVELFORGE_CONFIG) }
if ($NoAutopilot) { $Arguments += '--no-autopilot' }
& $Binary @Arguments
exit $LASTEXITCODE
