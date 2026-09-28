package scripts

import (
	"fmt"
	"strings"
)

// processWatchdogTemplate gira sullo studente: ogni 5s confronta i
// processi correnti con la denylist. Quando un nome processo della
// denylist appare per la prima volta, POSTa un evento.
//
// Segnaposto:
//
//	__IP_DOCENTE__   IP Planck
//	__PORTA_WEB__    porta web
const processWatchdogTemplate = `# ============================================================
# Planck watchdog Process - PowerShell 5.1 polling 5s
# ============================================================
# Genera un evento "started" la prima volta che un processo della
# denylist appare nei processi attivi (e "stopped" alla scomparsa).
# La denylist e' hardcoded — la versione configurabile arrivera'
# quando il plugin avra' una sezione Settings UI dedicata.
# ============================================================

$plancUrl = "http://__IP_DOCENTE__:__PORTA_WEB__/api/watchdog/event"

# Denylist case-insensitive (config docente). I nomi senza .exe matchano
# anche con .exe.
$denyList = @(__DENY_LIST__)

function Strip-Exe([string]$s) {
    $x = $s.ToLower()
    if ($x.EndsWith('.exe')) { return $x.Substring(0, $x.Length - 4) }
    return $x
}

# Pre-normalizziamo la denylist UNA VOLTA: Get-Process restituisce Name
# senza .exe (es. "cmd", "powershell"), mentre la denylist puo' avere
# .exe ("cmd.exe"). Senza questa normalizzazione il -contains falliva
# sempre e nessun evento "started" veniva mai inviato.
$denyListNorm = @()
foreach ($d in $denyList) { $denyListNorm += Strip-Exe $d }

function Test-Suspect($procName) {
    $clean = Strip-Exe $procName
    return $denyListNorm -contains $clean
}

function Send-Event($action, $proc) {
    $payload = @{
        plugin  = 'process'
        payload = @{
            action = $action
            name   = $proc.Name
            pid    = $proc.Id
        }
    } | ConvertTo-Json -Compress -Depth 4
    try {
        Invoke-RestMethod -Uri $plancUrl -Method POST -Body $payload -ContentType "application/json" -TimeoutSec 3 | Out-Null
    } catch {}
}

$heartbeatUrl = "http://__IP_DOCENTE__:__PORTA_WEB__/api/watchdog/heartbeat"
function Send-Heartbeat {
    try {
        Invoke-RestMethod -Uri $heartbeatUrl -Method POST -Body '{"plugin":"process"}' -ContentType "application/json" -TimeoutSec 3 | Out-Null
    } catch {}
}

# Snapshot iniziale: i processi gia' presenti al boot (eg cmd lanciato
# dal docente per debug) non sono "started".
$baseline = @{}
foreach ($p in Get-Process) {
    if (Test-Suspect $p.Name) { $baseline[$p.Id] = $p }
}

$heartbeatEvery = 1  # ogni tick da 5s -> heartbeat ogni 5s (tempo reale)
$stopFlag = Join-Path $env:TEMP 'planck_stop.flag'
$tick = 0
while ($true) {
    if (Test-Path $stopFlag) { exit 0 }
    Start-Sleep -Seconds 5
    if (Test-Path $stopFlag) { exit 0 }
    $current = @{}
    foreach ($p in Get-Process) {
        if (Test-Suspect $p.Name) {
            $current[$p.Id] = $p
            if (-not $baseline.ContainsKey($p.Id)) {
                Send-Event 'started' $p
            }
        }
    }
    foreach ($key in @($baseline.Keys)) {
        if (-not $current.ContainsKey($key)) {
            Send-Event 'stopped' $baseline[$key]
        }
    }
    $baseline = $current
    $tick++
    if ($tick % $heartbeatEvery -eq 0) { Send-Heartbeat }
}
`

// WatchdogProcessScript ritorna lo script PowerShell del plugin Process
// con IP/porta del docente sostituiti + denylist iniettata dalla config.
func WatchdogProcessScript(ipDocente string, portaWeb int, denyList []string) string {
	return strings.NewReplacer(
		"__IP_DOCENTE__", ipDocente,
		"__PORTA_WEB__", fmt.Sprintf("%d", portaWeb),
		"__DENY_LIST__", psStringArray(lowercase(denyList)),
	).Replace(processWatchdogTemplate)
}
