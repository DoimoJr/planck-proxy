// Package classify contiene le liste di domini e la funzione di
// classificazione del traffico in tre categorie: ai / sistema / utente.
//
// Match per sottostringa case-insensitive (es. "openai.com" cattura
// "chat.openai.com", "api.openai.com", ecc.). Stesso comportamento di v1
// (vedi domains.js originale, qui portato verbatim a Go).
//
// L'ordine di priorita' e' AI > sistema > utente: un dominio che matcha
// sia un pattern AI sia un pattern sistema viene classificato come AI.
//
// In Phase 5 (auto-classification AI) le liste verranno spostate in file
// JSON sotto data/ e potranno essere aggiornate via sync upstream + lista
// locale del docente. Per Phase 1 sono hardcoded come in v1.
package classify

import "strings"

// Tipo e' la categoria assegnata a un dominio dopo la classificazione.
type Tipo string

const (
	TipoAI      Tipo = "ai"
	TipoSistema Tipo = "sistema"
	TipoUtente  Tipo = "utente"
)

// dominiAILegacyHardcoded e' rimasta come riferimento storico per
// l'iniziale lista hardcoded di v1. La lista corrente vive ora in
// `data/ai-domains.txt` (embedded in `embedded_ai_domains.txt` per
// fallback) e viene caricata da `source.go` via `init()` +
// pull remote opzionale tramite `RefreshAIList`.
//
// I chiamanti devono usare `classify.AIDomains()` (snapshot del
// puntatore atomico) invece di una variabile globale.
var dominiAILegacyHardcoded = []string{
	// --- Chatbot principali (USA/Europa) ---
	"openai.com", "chatgpt.com",
	"anthropic.com", "claude.ai",
	"gemini.google.com", "bard.google.com", "aistudio.google.com", "notebooklm.google",
	"copilot.microsoft.com",
	"perplexity.ai", "you.com", "phind.com",
	"mistral.ai", "groq.com",
	"meta.ai",
	"grok.com",
	"pi.ai", "inflection.ai", "heypi.com",
	"duck.ai",
	"huggingface.co",
	"cohere.com",
	"poe.com", "character.ai",

	// --- Chatbot cinesi / asiatici ---
	"deepseek.com",
	"doubao.com",
	"yiyan.baidu.com", "ernie.baidu.com",
	"chatglm.cn", "bigmodel.cn",
	"kimi.com", "kimi.moonshot.cn", "moonshot.ai",
	"qwen.ai", "tongyi.aliyun.com",
	"minimax.io", "minimaxi.com", "hailuoai.com",
	"baichuan-ai.com",
	"01.ai", "lingyiwanwu.com",
	"alice.yandex.ru",

	// --- Wrapper / aggregatori di modelli ---
	"t3.chat", "chathub.gg",
	"openrouter.ai",
	"together.ai", "together.xyz",
	"fireworks.ai",
	"lobehub.com", "lobechat.com",
	"ollama.com",
	"replicate.com",
	"deepai.org", "venice.ai",

	// --- Chatbot generici (legacy ma ancora usati) ---
	"cleverbot.com", "simsimi.com",

	// --- Code assistants / "vibe coding" ---
	"cursor.com", "cursor.sh",
	"windsurf.com",
	"sourcegraph.com",
	"v0.dev", "v0.app",
	"bolt.new",
	"lovable.dev",
	"replit.com",
	"continue.dev",
	"aider.chat",
	"devin.ai", "manus.im", "genspark.ai",
	"blackbox.ai", "codeium.com", "tabnine.com",

	// --- Generazione immagini/video (solo se usabili per cheating testuale) ---
	"midjourney.com", "stability.ai",

	// --- Scrittura, parafrasi, essay writer ---
	"writesonic.com", "chatsonic.com",
	"jasper.ai", "copy.ai",
	"quillbot.com", "grammarly.com",
	"caktus.ai", "textero.io",
	"textcortex.com", "smodin.io",
	"neuroflash.com", "rytr.me",
	"wordtune.com", "paraphraser.io",
	"scribbr.com", "zerogpt.com",
	"wordvice.ai", "eduwriter.ai",
	"jenni.ai", "paperpal.com",

	// --- Risolutori compiti / matematica AI ---
	"wolframalpha.com",
	"photomath.com", "mathway.com", "symbolab.com",
	"socratic.org", "mathsolver.microsoft.com",
	"gauthmath.com", "gauth.ai",
	"brainly.com", "brainly.co",
	"mathgptpro.com", "math-gpt.org", "mathful.com", "math.bot",
	"chegg.com", "coursehero.com", "studocu.com",

	// --- AI su documenti / PDF chat ---
	"chatpdf.com", "pdf.ai", "askyourpdf.com",
	"humata.ai", "monica.im",

	// --- Estensioni / assistenti browser con backend web ---
	"maxai.me", "harpa.ai",
	"getmerlin.in", "merlin.foyer.work",
	"sider.ai", "arvin.chat",

	// --- Ricerca accademica AI ---
	"scispace.com", "consensus.app",
	"elicit.com", "elicit.org",
	"scite.ai", "researchrabbit.ai",
}

// PatternSistema: traffico di rumore (telemetria, ad tech, CMP, push services,
// CDN, update channel, ecc.) che non e' attivita' reale dello studente.
// La UI esclude queste entry dai conteggi per studente.
var PatternSistema = []string{
	// ---------- CDN / infrastruttura generica ----------
	"cdn.", "static.", "assets.", "beacons.", "beacon.",
	".cloudfront.net", ".akamaized.net", ".akamai.net",
	".cloudflare.com", ".fastly.net", ".fastly-edge.com",
	".edgecastcdn.net", ".jsdelivr.net",
	".azureedge.net", ".awswaf.com",

	// ---------- Google: APIs interne, push, thumbnails, beacons, update ----------
	".gstatic.com", ".googleapis.com", ".google-analytics.com",
	".googletagmanager.com", ".googlesyndication.com", ".googleadservices.com",
	".doubleclick.net", ".gvt1.com", ".gvt2.com", ".gvt3.com", ".1e100.net",
	".googleusercontent.com", ".googlevideo.com",
	"clients6.google.com",
	"mtalk.google.com", "play.google.com", "android.clients.google.com",
	"clients1.google.com", "clients2.google.com", "clients3.google.com", "clients4.google.com",
	"safebrowsing.googleapis.com", "passwordsleakcheck-pa.googleapis.com",
	"update.googleapis.com", "content-autofill.googleapis.com",
	"optimizationguide-pa.googleapis.com", "oauthaccountmanager.googleapis.com",
	"signaler-pa.googleapis.com", "jnn-pa.googleapis.com",
	"lensfrontend-pa.googleapis.com", "accountcapabilities-pa.googleapis.com",
	"chromewebstore.googleapis.com",
	"csp.withgoogle.com",
	"adservice.google.", "consent.google.",
	"sb-ssl.google.com",
	// API/servizi innescati dalla sessione loggata (non "attivita' studente")
	"apis.google.com", "ogs.google.com", "lh3.google.com",
	"accounts.youtube.com", "contacts.google.com",
	"mail-ads.google.com", "takeout.google.com",
	"drive.usercontent.google.com",

	// ---------- Microsoft / Windows / Edge (telemetria, update, servizi) ----------
	".msftconnecttest.com", ".msftncsi.com",
	".windowsupdate.com", ".update.microsoft.com",
	".events.data.microsoft.com", "events.data.msn.", ".telemetry.microsoft.com",
	".delivery.mp.microsoft.com", ".dsp.mp.microsoft.com",
	".licensing.mp.microsoft.com", ".displaycatalog.mp.microsoft.com",
	".checkappexec.microsoft.com",
	".microsoft.com",
	".msedge.net", ".msn.com", ".office.com", ".office365.com",
	".live.com", ".outlook.com", ".skype.com",
	".azure.com",
	".windows.com",
	"cxcs.microsoft.net", ".gfx.ms",
	"default.exp-tas.com",
	"config.edge.skype.com",
	"tile-service.weather.microsoft.com",

	// ---------- Mozilla / Firefox (telemetria, push, settings, Pocket, ads) ----------
	".mozilla.org", ".mozilla.com", ".mozilla.net", ".firefox.com",
	".mozgcp.net",
	"mozilla-ohttp.fastly-edge.com",

	// ---------- Apple infrastruttura ----------
	".apple.com", ".icloud.com", ".mzstatic.com",

	// ---------- Auth / SSO / OCSP / certificati ----------
	"accounts.google.", "login.", "auth.", "oauth.", "sso.",
	"id.", ".auth0.com", ".okta.com",
	"ocsp.", "crl.", ".digicert.com", ".letsencrypt.org", ".lencr.org",
	".verisign.com", ".sectigo.com", ".usertrust.com", ".pki.goog",

	// ---------- DNS e rete ----------
	"dns.", "ntp.", "wpad", "isatap",

	// ---------- Analytics / RUM / observability ----------
	"analytics.", "telemetry.", "metrics.", "stats.",
	"logs.", "logger.", "tracker.", "tracking.",
	".hotjar.com", ".segment.com", ".mixpanel.com", ".amplitude.com",
	".sprig.com", ".imrworldwide.com",
	".newrelic.com", ".nr-data.net",
	"datadoghq.com", ".datadoghq-browser-agent.com",
	".signalfx.com", ".sentry.io",
	".contentsquare.net", ".geoedge.be",
	".intercom.io", ".intercomcdn.com",

	// ---------- Tracking pixel / social widget ----------
	".facebook.net", ".fbcdn.net",
	"ct.pinterest.com", ".pinimg.com",
	"bat.bing.com", "analytics.tiktok.com",

	// ---------- Consent Management Platforms ----------
	".usercentrics.eu", ".onetrust.com", ".cookielaw.org",
	".privacymanager.io", "privacy-proxy.", ".trustarc.com",

	// ---------- Ad tech / RTB / bid sync ----------
	".adsense.", ".adnxs.com", ".criteo.com", ".taboola.com", ".outbrain.com",
	".rubiconproject.com", ".adsrvr.org", ".pubmatic.com",
	".casalemedia.com", ".media.net", ".3lift.com",
	".smartadserver.com", ".bidswitch.net", ".demdex.net", ".everesttech.net",
	".360yield.com", ".openx.net", ".adform.net", ".yieldlab.net",
	".turn.com", ".a-mo.net", ".agkn.com", ".contextweb.com", ".fwmrm.net",
	".zeotap.com", ".rlcdn.com", ".dotomi.com", ".ipredictive.com",
	".lijit.com", ".sharethrough.com", ".seedtag.com", ".tremorhub.com",
	".trustedstack.com", ".unrulymedia.com", ".loopme.me", ".1rx.io",
	".onetag-sys.com", ".stackadapt.com", ".px-cloud.net",
	".sitescout.com", ".simpli.fi", ".smaato.net", ".tapad.com",
	".bidr.io", ".teads.tv", ".amazon-adsystem.com", ".creativecdn.com",
	".adkernel.com", ".33across.com", ".omnitagjs.com", ".postrelease.com",
	"bttrack.com", ".t13.io", ".4dex.io", ".copper6.com", ".spot.im",
	"id5-sync.com", ".relevant-digital.com", ".adtrafficquality.google",
	"onetag-sys.com", "ad4m.at",
	".securitytrfx.com",
	".safeframe.googlesyndication.com",
	".platinumai.net", ".vistarsagency.com", ".company-target.com",
	".rfihub.com", ".ctnsnet.com",
	".yahoo.com", "ad-delivery.net",

	// ---------- Adobe check-ins / DTM (Reader, Acrobat, Experience Platform) ----------
	"acroipm", "armmf.adobe.com", "ardownload", ".adobedtm.com",
	"aepxlg.adobe.com",

	// ---------- Software update / dev tool channels ----------
	".vscode-cdn.net", "update.code.visualstudio.com",
	"marketplace.visualstudio.com", ".vsassets.io",
	"services.gradle.org",
	"code.jquery.com", "cdnjs.cloudflare.com",

	// ---------- Canva / productivity telemetry ----------
	"telemetry.canva.com", "static.canva.com",
	"chunk-composing.canva.com", "avatar.canva.com",
	"template.canva.com", ".canva-apps.com",

	// ---------- Endpoint security / sandbox antimalware (rumore) ----------
	".bromium-online.com",

	// ---------- Telemetria scuola/PA italiana ----------
	"analytics.istruzione.it", "analytics-1.istruzione.it",
	"analytics-2.istruzione.it", "analytics1.istruzione.it",

	// ---------- Privacy sandbox / altri ----------
	".privacysandboxservices.com",
	"cloud.dell.com", "csgdtm-svc-agent.dell.com",
	"cdn.ampproject.org",
	"api-engage-eu.sitecorecloud.io", ".sitecorecloud.io",
}

// Classifica restituisce la categoria di appartenenza di un dominio.
//
// Match per sottostringa case-insensitive. Priorita': AI > sistema > utente.
// Ritorna TipoUtente quando nessun pattern combacia.
//
//	classify.Classifica("chat.openai.com")    // -> TipoAI
//	classify.Classifica("watson.telemetry.microsoft.com") // -> TipoSistema
//	classify.Classifica("www.example.com")    // -> TipoUtente
func Classifica(dominio string) Tipo {
	d := strings.ToLower(dominio)
	for _, ai := range AIDomains() {
		if strings.Contains(d, ai) {
			return TipoAI
		}
	}
	for _, pat := range PatternSistema {
		if strings.Contains(d, pat) {
			return TipoSistema
		}
	}
	return TipoUtente
}
