package scripts

import (
	"fmt"
	"strings"
)

const networkWatchdogTemplate = `# ============================================================
# Planck watchdog Network - PowerShell 5.1 polling 5s
# ============================================================
# Genera un evento "added" la prima volta che una nuova interfaccia
# di rete appare in stato "Up" rispetto al baseline iniziale.
# Il baseline e' fatto al boot dello script: tutte le interfacce
# Up al momento del proxy_on diventano "trusted".
# ============================================================

$plancUrl = "http://__IP_DOCENTE__:__PORTA_WEB__/api/watchdog/event"

# Pattern di nomi interfaccia "sospetti" (substring case-insensitive
# nell'InterfaceDescription). Iniettati dalla config del docente.
$suspiciousPatterns = @(__SUSPICIOUS_PATTERNS__)
# Pattern di nomi da skippare completamente (mai allarmare).
$ignorePatterns = @(__IGNORE_PATTERNS__)

function Test-Match([string]$desc, [string[]]$patterns) {
    if (-not $desc) { return $false }
    foreach ($p in $patterns) {
        if ($desc -match [regex]::Escape($p)) { return $true }
    }
    return $false
}

function Get-UpAdapters {
    Get-NetAdapter -ErrorAction SilentlyContinue |
        Where-Object { $_.Status -eq 'Up' } |
        Where-Object { -not (Test-Match $_.InterfaceDescription $ignorePatterns) } |
        Select-Object Name, InterfaceDescription, MediaType, MacAddress, ifIndex
}

function Send-Event($action, $adapter, $suspicious) {
    $payload = @{
        plugin  = 'network'
        payload = @{
            action      = $action
            name        = $adapter.Name
            description = $adapter.InterfaceDescription
            mediaType   = $adapter.MediaType
            mac         = $adapter.MacAddress
            suspicious  = [bool]$suspicious
        }
    } | ConvertTo-Json -Compress -Depth 4
    try {
        Invoke-RestMethod -Uri $plancUrl -Method POST -Body $payload -ContentType "application/json" -TimeoutSec 3 | Out-Null
    } catch {}
}

$heartbeatUrl = "http://__IP_DOCENTE__:__PORTA_WEB__/api/watchdog/heartbeat"
function Send-Heartbeat {
    try {
        Invoke-RestMethod -Uri $heartbeatUrl -Method POST -Body '{"plugin":"network"}' -ContentType "application/json" -TimeoutSec 3 | Out-Null
    } catch {}
}

# Baseline iniziale: tutto cio' che e' gia' Up al boot dello script
# diventa "trusted" e non genera eventi. Solo le NUOVE interfacce
# allarmano.
$baseline = @{}
foreach ($a in Get-UpAdapters) {
    $baseline[$a.ifIndex] = $a
}

$heartbeatEvery = 1  # ogni tick da 5s -> heartbeat ogni 5s (tempo reale)
$stopFlag = Join-Path $env:TEMP 'planck_stop.flag'
$tick = 0
while ($true) {
    if (Test-Path $stopFlag) { exit 0 }
    Start-Sleep -Seconds 5
    if (Test-Path $stopFlag) { exit 0 }
    $current = @{}
    foreach ($a in Get-UpAdapters) {
        $current[$a.ifIndex] = $a
        if (-not $baseline.ContainsKey($a.ifIndex)) {
            $sus = Test-Match $a.InterfaceDescription $suspiciousPatterns
            Send-Event 'added' $a $sus
        }
    }
    foreach ($key in @($baseline.Keys)) {
        if (-not $current.ContainsKey($key)) {
            Send-Event 'removed' $baseline[$key] $false
        }
    }
    $baseline = $current
    $tick++
    if ($tick % $heartbeatEvery -eq 0) { Send-Heartbeat }
}
`

// WatchdogNetworkScript ritorna lo script PowerShell del plugin Network
// con IP/porta + pattern config sostituiti.
func WatchdogNetworkScript(ipDocente string, portaWeb int, suspicious, ignore []string) string {
	return strings.NewReplacer(
		"__IP_DOCENTE__", ipDocente,
		"__PORTA_WEB__", fmt.Sprintf("%d", portaWeb),
		"__SUSPICIOUS_PATTERNS__", psStringArray(suspicious),
		"__IGNORE_PATTERNS__", psStringArray(ignore),
	).Replace(networkWatchdogTemplate)
}
