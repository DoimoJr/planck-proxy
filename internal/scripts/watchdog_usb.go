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
# ============================================================

$plancUrl = "http://__IP_DOCENTE__:__PORTA_WEB__/api/watchdog/event"

# Classi PnP da ignorare (device sempre presenti, non interessanti).
$ignoredClasses = @(
    'HIDClass', 'Mouse', 'Keyboard',
    'USB',
    'Bluetooth',
    'AudioEndpoint', 'MEDIA',
    'System', 'Computer', 'Processor', 'Battery',
    'DiskDrive', 'Volume',
    'Net'
)

function Get-InterestingPnp {
    Get-PnpDevice -PresentOnly -Status OK 2>$null |
        Where-Object { $_.Class -and ($_.Class -notin $ignoredClasses) } |
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
// IP/porta del docente sostituiti. Usato dall'endpoint
// /api/scripts/watchdog/usb.ps1 per servirlo agli studenti.
func WatchdogUsbScript(ipDocente string, portaWeb int) string {
	return strings.NewReplacer(
		"__IP_DOCENTE__", ipDocente,
		"__PORTA_WEB__", fmt.Sprintf("%d", portaWeb),
	).Replace(usbWatchdogTemplate)
}
