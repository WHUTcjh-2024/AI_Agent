[CmdletBinding()]
param(
    [ValidateSet('all', 'offline', 'integration')]
    [string]$Suite = 'all',
    [string]$OutputDirectory = '',
    [switch]$NoRace,
    [switch]$KeepServices
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $repoRoot 'infrastructure/docker/docker-compose.eval.yml'
$startedServices = $false
$exitCode = 2
$savedPostgres = $env:ASKU_EVAL_POSTGRES_URL
$savedRedis = $env:ASKU_EVAL_REDIS_ADDR
$savedRedisPassword = $env:ASKU_EVAL_REDIS_PASSWORD

try {
    Get-Command go -ErrorAction Stop | Out-Null
    if ($Suite -ne 'offline') {
        Get-Command docker -ErrorAction Stop | Out-Null
        $existing = @(& docker compose -f $composeFile ps -q)
        if ($LASTEXITCODE -ne 0) { throw 'Unable to inspect evaluation services.' }
        $startedServices = $existing.Count -eq 0
        & docker compose -f $composeFile up -d --wait
        if ($LASTEXITCODE -ne 0) { throw 'Evaluation services failed to become healthy.' }
        $env:ASKU_EVAL_POSTGRES_URL = 'postgres://asku_eval:asku_eval_local@127.0.0.1:15433/asku_eval?sslmode=disable'
        $env:ASKU_EVAL_REDIS_ADDR = '127.0.0.1:16380'
        $env:ASKU_EVAL_REDIS_PASSWORD = $null
    }
    if (-not $OutputDirectory) {
        $OutputDirectory = Join-Path $repoRoot ('evals/reports/' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssfffffffZ'))
    } else {
        $OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
    }
    Push-Location (Join-Path $repoRoot 'backend')
    try {
        $evalArgs = @('run', './cmd/eval', '--suite', $Suite, '--root', $repoRoot, '--out', $OutputDirectory)
        if (-not $NoRace) { $evalArgs += '--race' }
        & go @evalArgs
        $exitCode = $LASTEXITCODE
    } finally {
        Pop-Location
    }
} finally {
    $env:ASKU_EVAL_POSTGRES_URL = $savedPostgres
    $env:ASKU_EVAL_REDIS_ADDR = $savedRedis
    $env:ASKU_EVAL_REDIS_PASSWORD = $savedRedisPassword
    if ($startedServices -and -not $KeepServices) {
        & docker compose -f $composeFile down
        if ($LASTEXITCODE -ne 0) { Write-Warning 'Evaluation services could not be stopped.'; $exitCode = 2 }
    }
}

exit $exitCode
