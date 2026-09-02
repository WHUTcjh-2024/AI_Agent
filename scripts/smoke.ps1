[CmdletBinding()]
param(
    [string]$BaseUrl = 'http://localhost:18080',
    [string]$AdminToken = 'asku-local-admin-do-not-use-in-production'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$base = $BaseUrl.TrimEnd('/')
$sessionId = $null
$headers = $null

try {
    $health = Invoke-RestMethod -Uri "$base/healthz"
    if ($health.status -ne 'ok') {
        throw "Backend is not healthy: $($health.status)"
    }

    $loginBody = @{ externalId = 'integration-tester'; nickname = '联调测试' } | ConvertTo-Json -Compress
    $login = Invoke-RestMethod -Method Post -Uri "$base/v1/auth/dev-login" -ContentType 'application/json' -Body $loginBody
    $headers = @{ Authorization = "Bearer $($login.accessToken)" }

    $me = Invoke-RestMethod -Headers $headers -Uri "$base/v1/me"
    $sessionBody = @{ title = '自动联调会话' } | ConvertTo-Json -Compress
    $session = Invoke-RestMethod -Method Post -Headers $headers -Uri "$base/v1/sessions" -ContentType 'application/json' -Body $sessionBody
    $sessionId = $session.id

    $messageId = "msg_$([guid]::NewGuid().ToString('N'))"
    $sendHeaders = @{
        Authorization = $headers.Authorization
        'Idempotency-Key' = [guid]::NewGuid().ToString()
    }
    $messageBody = @{ question = '官网搜索测试'; userMessageId = $messageId } | ConvertTo-Json -Compress
    $accepted = Invoke-RestMethod -Method Post -Headers $sendHeaders -Uri "$base/v1/sessions/$sessionId/messages" -ContentType 'application/json' -Body $messageBody

    $curl = (Get-Command curl.exe -ErrorAction Stop).Source
    $sse = (& $curl -sS -N --max-time 30 -H "Authorization: Bearer $($login.accessToken)" "$base/v1/runs/$($accepted.run.id)/events?after=0") -join "`n"
    if ($LASTEXITCODE -ne 0) {
        throw "SSE request failed with curl exit code $LASTEXITCODE"
    }
    $eventTypes = [regex]::Matches($sse, '(?m)^event:\s*(.+)$') | ForEach-Object { $_.Groups[1].Value.Trim() }
    $requiredEvents = @('run.started', 'route.resolved', 'retrieval.started', 'retrieval.completed', 'sources.updated', 'generation.started', 'message.delta', 'message.completed', 'run.completed')
    foreach ($required in $requiredEvents) {
        if ($required -notin $eventTypes) {
            throw "SSE stream is missing required event: $required"
        }
    }

    $history = Invoke-RestMethod -Headers $headers -Uri "$base/v1/sessions/$sessionId/messages"
    $assistant = @($history.messages | Where-Object role -eq 'assistant' | Select-Object -Last 1)
    if ($history.messages.Count -ne 2 -or $assistant.Count -ne 1 -or $assistant[0].citations.Count -lt 1) {
        throw 'Final assistant message or citations were not persisted.'
    }
    $source = Invoke-RestMethod -Headers $headers -Uri "$base/v1/sources/$($assistant[0].citations[0].sourceId)"

    $adminRuns = $null
    if ($AdminToken) {
        $admin = Invoke-RestMethod -Headers @{ Authorization = "Bearer $AdminToken" } -Uri "$base/v1/admin/overview"
        $adminRuns = $admin.quality.runs
    }

    [pscustomobject]@{
        Health = $health.status
        User = $me.id
        Run = $accepted.run.id
        Events = $eventTypes.Count
        Messages = $history.messages.Count
        Citations = $assistant[0].citations.Count
        Source = $source.title
        AdminRuns = $adminRuns
    }
}
finally {
    if ($sessionId -and $headers) {
        try {
            Invoke-RestMethod -Method Delete -Headers $headers -Uri "$base/v1/sessions/$sessionId" | Out-Null
        }
        catch {
            Write-Warning "Unable to remove smoke-test session $sessionId`: $($_.Exception.Message)"
        }
    }
}
