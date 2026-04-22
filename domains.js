/**
 * @file Liste domini per la classificazione del traffico.
 *
 * Unica fonte di verita' per le due categorie principali:
 * - `DOMINI_AI`: pattern di servizi di chatbot/assistenti/code AI/ricerca
 *   accademica AI/ecc. I match generano un banner di allarme in UI.
 * - `PATTERN_SISTEMA`: traffico di rumore (telemetria, ad tech, CMP, push
 *   services, update channel) che non e' attivita' reale dello studente.
 *   La UI esclude queste entry dai conteggi per-studente per non falsare il
 *   segnale.
 *
 * La terza categoria ("utente") e' la default quando nessun pattern combacia.
 *
 * **Match per sottostringa**: inserire "openai.com" cattura `chat.openai.com`,
 * `api.openai.com`, ecc. Scegliere il pattern piu' corto ma specifico possibile.
 *
 * Queste liste vengono esposte al client via `/api/config` ma il match
 * definitivo avviene sempre server-side in `classifica()`.
 */

const DOMINI_AI = [
    // --- Chatbot principali (USA/Europa) ---
    'openai.com', 'chatgpt.com',
    'anthropic.com', 'claude.ai',
    'gemini.google.com', 'bard.google.com', 'aistudio.google.com', 'notebooklm.google',
    'copilot.microsoft.com',
    'perplexity.ai', 'you.com', 'phind.com',
    'mistral.ai', 'groq.com',
    'meta.ai',
    'grok.com',
    'pi.ai', 'inflection.ai', 'heypi.com',
    'duck.ai',
    'huggingface.co',
    'cohere.com',
    'poe.com', 'character.ai',

    // --- Chatbot cinesi / asiatici ---
    'deepseek.com',
    'doubao.com',
    'yiyan.baidu.com', 'ernie.baidu.com',
    'chatglm.cn', 'bigmodel.cn',
    'kimi.com', 'kimi.moonshot.cn', 'moonshot.ai',
    'qwen.ai', 'tongyi.aliyun.com',
    'minimax.io', 'minimaxi.com', 'hailuoai.com',
    'baichuan-ai.com',
    '01.ai', 'lingyiwanwu.com',
    'alice.yandex.ru',

    // --- Wrapper / aggregatori di modelli ---
    't3.chat', 'chathub.gg',
    'openrouter.ai',
    'together.ai', 'together.xyz',
    'fireworks.ai',
    'lobehub.com', 'lobechat.com',
    'ollama.com',
    'replicate.com',
    'deepai.org', 'venice.ai',

    // --- Chatbot generici (legacy ma ancora usati) ---
    'cleverbot.com', 'simsimi.com',

    // --- Code assistants / "vibe coding" ---
    'cursor.com', 'cursor.sh',
    'windsurf.com',
    'sourcegraph.com',
    'v0.dev', 'v0.app',
    'bolt.new',
    'lovable.dev',
    'replit.com',
    'continue.dev',
    'aider.chat',
    'devin.ai', 'manus.im', 'genspark.ai',
    'blackbox.ai', 'codeium.com', 'tabnine.com',

    // --- Generazione immagini/video (solo se usabili per cheating testuale) ---
    'midjourney.com', 'stability.ai',

    // --- Scrittura, parafrasi, essay writer ---
    'writesonic.com', 'chatsonic.com',
    'jasper.ai', 'copy.ai',
    'quillbot.com', 'grammarly.com',
    'caktus.ai', 'textero.io',
    'textcortex.com', 'smodin.io',
    'neuroflash.com', 'rytr.me',
    'wordtune.com', 'paraphraser.io',
    'scribbr.com', 'zerogpt.com',
    'wordvice.ai', 'eduwriter.ai',
    'jenni.ai', 'paperpal.com',

    // --- Risolutori compiti / matematica AI ---
    'wolframalpha.com',
    'photomath.com', 'mathway.com', 'symbolab.com',
    'socratic.org', 'mathsolver.microsoft.com',
    'gauthmath.com', 'gauth.ai',
    'brainly.com', 'brainly.co',
    'mathgptpro.com', 'math-gpt.org', 'mathful.com', 'math.bot',
    'chegg.com', 'coursehero.com', 'studocu.com',

    // --- AI su documenti / PDF chat ---
    'chatpdf.com', 'pdf.ai', 'askyourpdf.com',
    'humata.ai', 'monica.im',

    // --- Estensioni / assistenti browser con backend web ---
    'maxai.me', 'harpa.ai',
    'getmerlin.in', 'merlin.foyer.work',
    'sider.ai', 'arvin.chat',

    // --- Ricerca accademica AI ---
    'scispace.com', 'consensus.app',
    'elicit.com', 'elicit.org',
    'scite.ai', 'researchrabbit.ai',
];

// ============================================================
// PATTERN_SISTEMA: traffico "rumore" che parte automaticamente
// (telemetria, servizi push, ad tech, CMP, certificati, CDN, update channel, ecc.)
// NON e' attivita' reale dello studente: viene ancora registrato e mostrato
// nella sidebar "Sistema" per trasparenza, ma escluso dai conteggi per studente.
// ============================================================
const PATTERN_SISTEMA = [
    // ---------- CDN / infrastruttura generica ----------
    'cdn.', 'static.', 'assets.', 'beacons.', 'beacon.',
    '.cloudfront.net', '.akamaized.net', '.akamai.net',
    '.cloudflare.com', '.fastly.net', '.fastly-edge.com',
    '.edgecastcdn.net', '.jsdelivr.net',
    '.azureedge.net', '.awswaf.com',

    // ---------- Google: APIs interne, push, thumbnails, beacons, update ----------
    '.gstatic.com', '.googleapis.com', '.google-analytics.com',
    '.googletagmanager.com', '.googlesyndication.com', '.googleadservices.com',
    '.doubleclick.net', '.gvt1.com', '.gvt2.com', '.gvt3.com', '.1e100.net',
    '.googleusercontent.com', '.googlevideo.com',
    'clients6.google.com',
    'mtalk.google.com', 'play.google.com', 'android.clients.google.com',
    'clients1.google.com', 'clients2.google.com', 'clients3.google.com', 'clients4.google.com',
    'safebrowsing.googleapis.com', 'passwordsleakcheck-pa.googleapis.com',
    'update.googleapis.com', 'content-autofill.googleapis.com',
    'optimizationguide-pa.googleapis.com', 'oauthaccountmanager.googleapis.com',
    'signaler-pa.googleapis.com', 'jnn-pa.googleapis.com',
    'lensfrontend-pa.googleapis.com', 'accountcapabilities-pa.googleapis.com',
    'chromewebstore.googleapis.com',
    'csp.withgoogle.com',
    'adservice.google.', 'consent.google.',
    'sb-ssl.google.com',
    // Google: API/servizi innescati dalla sessione loggata (non "attivita' studente")
    'apis.google.com', 'ogs.google.com', 'lh3.google.com',
    'accounts.youtube.com', 'contacts.google.com',
    'mail-ads.google.com', 'takeout.google.com',
    'drive.usercontent.google.com',

    // ---------- Microsoft / Windows / Edge (telemetria, update, servizi) ----------
    '.msftconnecttest.com', '.msftncsi.com',
    '.windowsupdate.com', '.update.microsoft.com',
    '.events.data.microsoft.com', '.telemetry.microsoft.com',
    '.delivery.mp.microsoft.com', '.dsp.mp.microsoft.com',
    '.licensing.mp.microsoft.com', '.displaycatalog.mp.microsoft.com',
    '.checkappexec.microsoft.com',
    '.microsoft.com',
    '.msedge.net', '.msn.com', '.office.com', '.office365.com',
    '.live.com', '.outlook.com', '.skype.com',
    '.azure.com',
    '.windows.com',
    'cxcs.microsoft.net',
    'default.exp-tas.com',
    'config.edge.skype.com',
    'tile-service.weather.microsoft.com',

    // ---------- Mozilla / Firefox (telemetria, push, settings, Pocket, ads) ----------
    '.mozilla.org', '.mozilla.com', '.mozilla.net', '.firefox.com',
    '.mozgcp.net',
    'mozilla-ohttp.fastly-edge.com',

    // ---------- Apple infrastruttura ----------
    '.apple.com', '.icloud.com', '.mzstatic.com',

    // ---------- Auth / SSO / OCSP / certificati ----------
    'accounts.google.', 'login.', 'auth.', 'oauth.', 'sso.',
    'id.', '.auth0.com', '.okta.com',
    'ocsp.', 'crl.', '.digicert.com', '.letsencrypt.org', '.lencr.org',
    '.verisign.com', '.sectigo.com', '.usertrust.com', '.pki.goog',

    // ---------- DNS e rete ----------
    'dns.', 'ntp.', 'wpad', 'isatap',

    // ---------- Analytics / RUM / observability ----------
    'analytics.', 'telemetry.', 'metrics.', 'stats.',
    'logs.', 'logger.', 'tracker.', 'tracking.',
    '.hotjar.com', '.segment.com', '.mixpanel.com', '.amplitude.com',
    '.newrelic.com', '.nr-data.net',
    'datadoghq.com', '.datadoghq-browser-agent.com',
    '.signalfx.com', '.sentry.io',
    '.contentsquare.net', '.geoedge.be',
    '.intercom.io', '.intercomcdn.com',

    // ---------- Tracking pixel / social widget ----------
    '.facebook.net', '.fbcdn.net',
    'ct.pinterest.com', '.pinimg.com',
    'bat.bing.com', 'analytics.tiktok.com',

    // ---------- Consent Management Platforms ----------
    '.usercentrics.eu', '.onetrust.com', '.cookielaw.org',
    '.privacymanager.io', 'privacy-proxy.',

    // ---------- Ad tech / RTB / bid sync ----------
    '.adsense.', '.adnxs.com', '.criteo.com', '.taboola.com', '.outbrain.com',
    '.rubiconproject.com', '.adsrvr.org', '.pubmatic.com',
    '.casalemedia.com', '.media.net', '.3lift.com',
    '.smartadserver.com', '.bidswitch.net', '.demdex.net', '.everesttech.net',
    '.360yield.com', '.openx.net', '.adform.net', '.yieldlab.net',
    '.turn.com', '.a-mo.net', '.agkn.com', '.contextweb.com', '.fwmrm.net',
    '.zeotap.com', '.rlcdn.com', '.dotomi.com', '.ipredictive.com',
    '.lijit.com', '.sharethrough.com', '.seedtag.com', '.tremorhub.com',
    '.trustedstack.com', '.unrulymedia.com', '.loopme.me', '.1rx.io',
    '.onetag-sys.com', '.stackadapt.com', '.px-cloud.net',
    '.sitescout.com', '.simpli.fi', '.smaato.net', '.tapad.com',
    '.bidr.io', '.teads.tv', '.amazon-adsystem.com', '.creativecdn.com',
    '.adkernel.com', '.33across.com', '.omnitagjs.com', '.postrelease.com',
    'bttrack.com', '.t13.io', '.4dex.io', '.copper6.com', '.spot.im',
    'id5-sync.com', '.relevant-digital.com', '.adtrafficquality.google',
    'onetag-sys.com', 'ad4m.at',
    '.securitytrfx.com',
    '.safeframe.googlesyndication.com',
    '.platinumai.net', '.vistarsagency.com', '.company-target.com',
    '.rfihub.com', '.ctnsnet.com',
    '.yahoo.com',

    // ---------- Adobe check-ins / DTM (Reader, Acrobat, Experience Platform) ----------
    'acroipm', 'armmf.adobe.com', 'ardownload', '.adobedtm.com',
    'aepxlg.adobe.com',

    // ---------- Software update / dev tool channels ----------
    '.vscode-cdn.net', 'update.code.visualstudio.com',
    'marketplace.visualstudio.com', '.vsassets.io',
    'services.gradle.org',
    'code.jquery.com', 'cdnjs.cloudflare.com',

    // ---------- Canva / productivity telemetry ----------
    'telemetry.canva.com', 'static.canva.com',
    'chunk-composing.canva.com', 'avatar.canva.com',
    'template.canva.com', '.canva-apps.com',

    // ---------- Privacy sandbox / altri ----------
    '.privacysandboxservices.com',
    'cloud.dell.com', 'csgdtm-svc-agent.dell.com',
    'cdn.ampproject.org',
    'api-engage-eu.sitecorecloud.io', '.sitecorecloud.io',
];

/**
 * Classifica un dominio in una delle tre categorie: `ai`, `sistema`, `utente`.
 * Ordine di priorita': AI > sistema > utente (i match AI vincono sempre,
 * anche se il dominio matcherebbe anche un pattern di sistema).
 *
 * @param {string} dominio - Hostname completo (es. `www.example.com`).
 * @returns {'ai'|'sistema'|'utente'}
 */
function classifica(dominio) {
    const d = dominio.toLowerCase();
    for (const ai of DOMINI_AI) {
        if (d.includes(ai)) return 'ai';
    }
    for (const pat of PATTERN_SISTEMA) {
        if (d.includes(pat)) return 'sistema';
    }
    return 'utente';
}

module.exports = { DOMINI_AI, PATTERN_SISTEMA, classifica };
