package scripts

import (
	"fmt"
	"strings"
)

// usbWatchdogTemplate e' il PowerShell che gira sul PC studente.
// Polling 5s, diff vs baseline, POST a /api/watchdog/event per ogni
// dispositivo USB nuovo.
//
// Segnaposto sostituiti in WatchdogUsbScript:
//
//	__IP_DOCENTE__   IP del PC Planck (dalla LAN del docente)
//	__PORTA_WEB__    porta del web server Planck (default 9999)
const usbWatchdogTemplate = `# ============================================================
# Planck watchdog USB - PowerShell 5.1 polling 5s
# ============================================================
# Genera eventi quando lo studente collega/scollega dispositivi USB
# di classe non sicura. Filtra HID/Mouse/Keyboard/Audio integrati.
# Le liste sono iniettate dal server al momento del download (dipendono
# dalla config del docente).
# ============================================================

$plancUrl = "http://__IP_DOCENTE__:__PORTA_WEB__/api/watchdog/event"

# Classi PnP da ignorare (config docente).
$ignoredClasses = @(__IGNORED_CLASSES__)
# Allowlist VID:PID (formato "1234:5678") per device legittimi.
$allowVidPid = @(__ALLOW_VID_PID__)

function Get-VidPid([string]$instanceId) {
    if ($instanceId -match 'VID_([0-9A-Fa-f]{4})&PID_([0-9A-Fa-f]{4})') {
        return ($matches[1] + ':' + $matches[2]).ToLower()
    }
    return ''
}

function Get-InterestingPnp {
    Get-PnpDevice -PresentOnly -Status OK 2>$null |
        Where-Object { $_.Class -and ($_.Class -notin $ignoredClasses) } |
        Where-Object { $vp = Get-VidPid $_.InstanceId; $vp -eq '' -or $allowVidPid -notcontains $vp } |
        Select-Object InstanceId, Class, FriendlyName
}

function Send-Event($action, $device) {
    $payload = @{
        plugin  = 'usb'
        payload = @{
            action     = $action
            instanceId = $device.InstanceId
            class      = $device.Class
            deviceName = $device.FriendlyName
        }
    } | ConvertTo-Json -Compress -Depth 4
    try {
        Invoke-RestMethod -Uri $plancUrl -Method POST -Body $payload -ContentType "application/json" -TimeoutSec 3 | Out-Null
    } catch {
        # Silenzio: rete temporaneamente irraggiungibile o Planck spento.
    }
}

# Snapshot iniziale: device gia' presenti al boot non sono "added".
$baseline = @{}
foreach ($d in Get-InterestingPnp) {
    $baseline[$d.InstanceId] = $d
}

while ($true) {
    Start-Sleep -Seconds 5
    $current = @{}
    foreach ($d in Get-InterestingPnp) {
        $current[$d.InstanceId] = $d
        if (-not $baseline.ContainsKey($d.InstanceId)) {
            Send-Event 'added' $d
        }
    }
    foreach ($key in @($baseline.Keys)) {
        if (-not $current.ContainsKey($key)) {
            Send-Event 'removed' $baseline[$key]
        }
    }
    $baseline = $current
}
`

// WatchdogUsbScript ritorna lo script PowerShell del plugin USB con
// IP/porta del docente sostituiti + denylist/allowlist iniettate
// dalla config del plugin in DB.
//
// `ignoredClasses` sono le classi PnP che il polling salta (default
// HID/Mouse/Audio/...).  `allowVidPid` sono coppie hex "1234:5678"
// (case-insensitive) per device legittimi del docente.
func WatchdogUsbScript(ipDocente string, portaWeb int, ignoredClasses, allowVidPid []string) string {
	return strings.NewReplacer(
		"__IP_DOCENTE__", ipDocente,
		"__PORTA_WEB__", fmt.Sprintf("%d", portaWeb),
		"__IGNORED_CLASSES__", psStringArray(ignoredClasses),
		"__ALLOW_VID_PID__", psStringArray(lowercase(allowVidPid)),
	).Replace(usbWatchdogTemplate)
}

// psStringArray formatta uno slice di stringhe come array PowerShell:
//
//	["a", "b", "c"]  →  'a','b','c'
//
// Singoli apici escapati con il pattern PowerShell `''`. Le stringhe
// vengono inserite nella sintassi `@(...)` del template.
func psStringArray(items []string) string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		s = strings.ReplaceAll(s, "'", "''")
		out = append(out, "'"+s+"'")
	}
	return strings.Join(out, ",")
}

// lowercase ritorna una copia di items con tutto in minuscolo. Usato
// per allowVidPid (Get-VidPid lo confronta in lowercase).
func lowercase(items []string) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = strings.ToLower(s)
	}
	return out
}
