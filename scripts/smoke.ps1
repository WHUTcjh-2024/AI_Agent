[CmdletBinding()]
param(
    [string]$BaseUrl = 'http://localhost:18080',
    [string]$AdminToken = 'asku-local-admin-do-not-use-in-production'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-SseEventPayload {
    param(
        [Parameter(Mandatory)] [string]$Stream,
        [Parameter(Mandatory)] [string]$EventType
    )

    $lines = $Stream -split "`n"
    for ($index = 0; $index -lt $lines.Count - 1; $index++) {
        if ($lines[$index].Trim() -ne "event: $EventType") {
            continue
        }
        $dataLine = $lines[$index + 1].Trim()
        if (-not $dataLine.StartsWith('data:')) {
            throw "SSE event $EventType has no data line."
        }
        return $dataLine.Substring(5).Trim() | ConvertFrom-Json
    }
    throw "SSE stream is missing event payload: $EventType"
}

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
    $messageBody = @{ question = '今年四六级什么时候报名？'; userMessageId = $messageId } | ConvertTo-Json -Compress
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

    $routePayload = Get-SseEventPayload -Stream $sse -EventType 'route.resolved'
    $retrievalStarted = Get-SseEventPayload -Stream $sse -EventType 'retrieval.started'
    $retrievalCompleted = Get-SseEventPayload -Stream $sse -EventType 'retrieval.completed'
    if ($routePayload.route -ne 'hybrid' -or $retrievalStarted.engine -ne 'hybrid' -or $retrievalCompleted.retrievalMode -ne 'hybrid') {
        throw 'Fresh question did not execute through the hybrid Agent route.'
    }
    if ($null -eq $retrievalCompleted.knowledgeStats -or $retrievalCompleted.knowledgeStats.configured -ne $false -or $null -eq $retrievalCompleted.searchStats) {
        throw 'Hybrid retrieval metadata does not show disabled Knowledge plus successful Web Search.'
    }
    if (@($retrievalCompleted.degradedCapabilities) -notcontains 'knowledge') {
        throw 'Hybrid retrieval did not expose the disabled Knowledge capability as degraded.'
    }

    $history = Invoke-RestMethod -Headers $headers -Uri "$base/v1/sessions/$sessionId/messages"
    $assistant = @($history.messages | Where-Object role -eq 'assistant' | Select-Object -Last 1)
    if ($history.messages.Count -ne 2 -or $assistant.Count -ne 1 -or $assistant[0].citations.Count -lt 1) {
        throw 'Final assistant message or citations were not persisted.'
    }
    $source = Invoke-RestMethod -Headers $headers -Uri "$base/v1/sources/$($assistant[0].citations[0].sourceId)"

    $adminRuns = $null
    $adminHybridRuns = $null
    if ($AdminToken) {
        $admin = Invoke-RestMethod -Headers @{ Authorization = "Bearer $AdminToken" } -Uri "$base/v1/admin/overview"
        $adminRuns = $admin.quality.runs
        $hybridMetric = @($admin.routes | Where-Object name -eq 'hybrid' | Select-Object -First 1)
        if ($hybridMetric.Count -ne 1 -or $hybridMetric[0].count -lt 1) {
            throw 'Admin overview did not aggregate the completed hybrid route.'
        }
        $adminHybridRuns = $hybridMetric[0].count
    }

    [pscustomobject]@{
        Health = $health.status
        User = $me.id
        Run = $accepted.run.id
        Route = $routePayload.route
        Retrieval = $retrievalStarted.engine
        KnowledgeConfigured = $retrievalCompleted.knowledgeStats.configured
        Events = $eventTypes.Count
        Messages = $history.messages.Count
        Citations = $assistant[0].citations.Count
        Source = $source.title
        AdminRuns = $adminRuns
        AdminHybridRuns = $adminHybridRuns
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
