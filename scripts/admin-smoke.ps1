#Requires -Version 7.0
[CmdletBinding()]
param(
    [string]$BackendUrl = 'http://127.0.0.1:18081',
    [string]$ConsoleUrl = 'http://127.0.0.1:18090',
    [string]$ConsolePassword = $env:ASKU_ADMIN_TEST_PASSWORD,
    [string]$AdminToken = 'asku-local-admin-do-not-use-in-production',
    [string]$ReportPath = '',
    [switch]$KeepSession
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if (-not $ConsolePassword) { throw 'Set ASKU_ADMIN_TEST_PASSWORD or provide -ConsolePassword.' }
$backend = $BackendUrl.TrimEnd('/')
$console = $ConsoleUrl.TrimEnd('/')
foreach ($address in @($backend, $console)) {
    $uri = [uri]$address
    if (-not $uri.IsLoopback -or $uri.Scheme -notin @('http', 'https')) {
        throw 'Run this smoke test against an isolated loopback environment.'
    }
}
$checks = [System.Collections.Generic.List[string]]::new()
$cleanupErrors = [System.Collections.Generic.List[string]]::new()
$failureMessage = $null
$cookies = [Microsoft.PowerShell.Commands.WebRequestSession]::new()
$sessionId = $null
$userHeaders = @{}
$passed = $false

function Assert-Check([bool]$Condition, [string]$Name) {
    if (-not $Condition) { throw "Check failed: $Name" }
    $checks.Add($Name)
}

function Request {
    param([string]$Url, [int]$Status = 200, [string]$Method = 'GET',
        [hashtable]$Headers = @{}, [object]$Body = $null, [switch]$WithCookies)
    $options = @{ Uri = $Url; Method = $Method; Headers = $Headers; SkipHttpErrorCheck = $true; TimeoutSec = 45 }
    if ($WithCookies) { $options.WebSession = $cookies }
    if ($null -ne $Body) {
        $options.ContentType = 'application/json; charset=utf-8'
        $options.Body = [Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json -Compress))
    }
    $response = Invoke-WebRequest @options
    if ([int]$response.StatusCode -ne $Status) {
        throw "Unexpected HTTP status for $Method $($Url.Split('?')[0]): $($response.StatusCode), expected $Status"
    }
    return $response
}

try {
    Request "$backend/healthz" | Out-Null
    Request "$console/healthz" | Out-Null
    Request "$console/api/overview" -Status 401 | Out-Null
    $checks.Add('anonymous access denied')
    Request "$console/api/session" -Method POST -Status 403 -Headers @{ Origin = 'https://untrusted.example' } -Body @{ password = $ConsolePassword } | Out-Null
    $checks.Add('cross-origin login denied')
    Request "$console/api/session" -Method POST -Status 401 -Headers @{ Origin = $console } -Body @{ password = 'incorrect-test-password' } | Out-Null
    $checks.Add('invalid password denied')
    $signIn = Request "$console/api/session" -Method POST -Headers @{ Origin = $console } -Body @{ password = $ConsolePassword } -WithCookies
    Assert-Check (($signIn.Headers['Set-Cookie'] -join ';') -match '(?i)HttpOnly' -and ($signIn.Headers['Set-Cookie'] -join ';') -match '(?i)SameSite=Strict') 'HttpOnly SameSite session cookie'
    Request "$console/api/session" -WithCookies | Out-Null

    $from = [DateTime]::UtcNow.AddDays(-1).ToString('yyyy-MM-dd')
    $to = [DateTime]::UtcNow.AddDays(1).ToString('yyyy-MM-dd')
    $query = "from=$from&to=$to"
    $before = (Request "$console/api/overview?$query" -WithCookies).Content | ConvertFrom-Json
    Request "$console/api/overview?from=invalid&to=$to" -Status 400 -WithCookies | Out-Null
    Request "$console/api/overview?schoolId=other-school" -Status 400 -WithCookies | Out-Null
    $checks.Add('date validation and query allowlist')

    $login = (Request "$backend/v1/auth/dev-login" -Method POST -Body @{ externalId = "admin-smoke-$([guid]::NewGuid().ToString('N'))"; nickname = 'Admin 联调' }).Content | ConvertFrom-Json
    $userHeaders = @{ Authorization = "Bearer $($login.accessToken)" }
    Request "$backend/v1/admin/overview" -Headers $userHeaders -Status 401 | Out-Null
    Request "$console/api/overview" -Headers $userHeaders -Status 401 | Out-Null
    $checks.Add('ordinary user token denied by backend and console')
    $session = (Request "$backend/v1/sessions" -Method POST -Status 201 -Headers $userHeaders -Body @{ title = 'Admin 自动联调' }).Content | ConvertFrom-Json
    $sessionId = $session.id
    $question = "今年校历怎么安排？联调编号 $([guid]::NewGuid().ToString('N'))"
    $accepted = (Request "$backend/v1/sessions/$sessionId/messages" -Method POST -Status 202 -Headers $userHeaders -Body @{ question = $question; userMessageId = "msg_$([guid]::NewGuid().ToString('N'))" }).Content | ConvertFrom-Json
    $stream = (Request "$backend/v1/runs/$($accepted.run.id)/events?after=0" -Headers $userHeaders).Content
    Assert-Check ($stream -match '(?m)^event: run.completed\r?$' -and $stream -match '(?m)^event: message.completed\r?$' -and $stream -notmatch '(?m)^event: run.failed') 'question completes through HTTP and SSE'

    $after = (Request "$console/api/overview?$query" -WithCookies).Content | ConvertFrom-Json
    Assert-Check ($after.engagement.questions -eq $before.engagement.questions + 1) 'question count increases by one'
    Assert-Check ($after.quality.runs -eq $before.quality.runs + 1 -and $after.quality.completedRuns -eq $before.quality.completedRuns + 1) 'completed run count increases by one'
    Assert-Check (@($after.topQuestions | Where-Object question -eq $question).Count -eq 1) 'question visible in admin aggregation'
    $direct = (Request "$backend/v1/admin/overview?$query" -Headers @{ Authorization = "Bearer $AdminToken" }).Content | ConvertFrom-Json
    foreach ($group in @('window', 'users', 'engagement', 'quality', 'performance', 'cost', 'routes', 'errorCodes', 'daily', 'topQuestions')) {
        Assert-Check (($after.$group | ConvertTo-Json -Depth 10 -Compress) -ceq ($direct.$group | ConvertTo-Json -Depth 10 -Compress)) "proxy matches backend $group"
    }
    foreach ($asset in @('/', '/app.js', '/api.js', '/metrics.js', '/render.js')) {
        $content = (Request "$console$asset").Content
        Assert-Check (-not $content.Contains($AdminToken) -and -not $content.Contains($ConsolePassword)) "no server credentials in public asset $asset"
    }
    Request "$console/api/session" -Method DELETE -Status 204 -Headers @{ Origin = $console } -WithCookies | Out-Null
    Request "$console/api/overview" -Status 401 -WithCookies | Out-Null
    $checks.Add('logout invalidates session')
    $passed = $true
}
catch {
    $failureMessage = $_.Exception.Message
    throw
}
finally {
    if ($sessionId -and -not ($KeepSession -and $passed)) {
        try { Request "$backend/v1/sessions/$sessionId" -Method DELETE -Status 204 -Headers $userHeaders | Out-Null }
        catch { $cleanupErrors.Add('Unable to delete the smoke-test session.') }
    }
    try { Request "$console/api/session" -Method DELETE -Status 204 -Headers @{ Origin = $console } -WithCookies | Out-Null }
    catch { $cleanupErrors.Add('Unable to invalidate the smoke-test console cookie.') }
    $report = [ordered]@{
        status = $(if ($passed -and $cleanupErrors.Count -eq 0) { 'passed' } else { 'failed' })
        generatedAt = [DateTime]::UtcNow.ToString('o')
        expectedProviderConfig = @{ llm = 'mock'; webSearch = 'mock'; knowledge = 'disabled'; data = 'isolated smoke fixtures' }
        checks = @($checks.ToArray())
        sessionRetained = [bool]($KeepSession -and $passed)
        failure = $failureMessage
        cleanupErrors = @($cleanupErrors.ToArray())
    }
    $json = $report | ConvertTo-Json -Depth 10
    if ($ReportPath) {
        $reportFile = [IO.Path]::GetFullPath($ReportPath)
        [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($reportFile)) | Out-Null
        [IO.File]::WriteAllText($reportFile, $json)
    }
    $json
}
if ($cleanupErrors.Count -gt 0) { throw 'Smoke-test cleanup failed; see the report.' }
