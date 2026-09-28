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
# Genera un evento "started" quando un processo della denylist compare,
# e "stopped" quando l'ultima sua istanza sparisce.
#
# Il conteggio e' per NOME, non per PID. I browser moderni sono
# multi-processo: ogni scheda di Firefox e' un firefox.exe distinto, e
# tracciare i PID produceva una raffica di eventi per ogni apertura.
# Misurato nella sessione 5BII del 2026-09-28: 988 eventi su 1000 erano
# firefox, con tre PID avviati nello stesso secondo e due terminati un
# minuto dopo. Il segnale utile in mezzo era cmd (6) e Taskmgr (4).
#
# Contando per nome, aprire il browser = un evento, chiuderlo = un altro.
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

function Send-Event($action, $nome, $istanze, $primoPid) {
    $payload = @{
        plugin  = 'process'
        payload = @{
            action  = $action
            name    = $nome
            pid     = $primoPid
            istanze = $istanze
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

# Conta le istanze per nome sospetto:
#   @{ 'firefox' = @{ n = 3; primoPid = 1284; nome = 'firefox' } }
#
# La chiave si chiama primoPid e non pid perche' $PID e' una variabile
# automatica di PowerShell (il processo corrente): un nome diverso evita
# ogni ambiguita' per chi legge. L'incremento passa da una variabile
# intermedia invece di $mappa[$k].n++, che su un hashtable annidato e'
# corretto ma si legge male e si rompe in silenzio se qualcuno lo tocca.
function Get-SospettiPerNome {
    $mappa = @{}
    foreach ($p in Get-Process) {
        if (Test-Suspect $p.Name) {
            $k = Strip-Exe $p.Name
            if ($mappa.ContainsKey($k)) {
                $voce = $mappa[$k]
                $voce.n = $voce.n + 1
            } else {
                $mappa[$k] = @{ n = 1; primoPid = $p.Id; nome = $p.Name }
            }
        }
    }
    return $mappa
}

# Snapshot iniziale: quello che gia' gira al boot (es. un cmd aperto dal
# docente) non conta come "started".
$baseline = Get-SospettiPerNome

$heartbeatEvery = 1  # ogni tick da 5s -> heartbeat ogni 5s (tempo reale)
$stopFlag = Join-Path $env:TEMP 'planck_stop.flag'
$tick = 0
while ($true) {
    if (Test-Path $stopFlag) { exit 0 }
    Start-Sleep -Seconds 5
    if (Test-Path $stopFlag) { exit 0 }
    $current = Get-SospettiPerNome
    # Comparsa: il nome non c'era e ora c'e' (0 -> N istanze).
    foreach ($k in @($current.Keys)) {
        if (-not $baseline.ContainsKey($k)) {
            Send-Event 'started' $current[$k].nome $current[$k].n $current[$k].primoPid
        }
    }
    # Scomparsa: era presente e ora non c'e' piu' nessuna istanza (N -> 0).
    foreach ($k in @($baseline.Keys)) {
        if (-not $current.ContainsKey($k)) {
            Send-Event 'stopped' $baseline[$k].nome 0 $baseline[$k].primoPid
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
