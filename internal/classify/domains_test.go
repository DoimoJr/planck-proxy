package classify

import "testing"

// TestClassificaAI verifica che i domini noti AI (substring match) siano
// classificati come TipoAI.
func TestClassificaAI(t *testing.T) {
	casi := []string{
		"chat.openai.com",
		"api.openai.com",
		"chatgpt.com",
		"claude.ai",
		"www.anthropic.com",
		"gemini.google.com",
		"copilot.microsoft.com",
		"perplexity.ai",
		"deepseek.com",
		"poe.com",
		"v0.dev",
		"cursor.com",
		"chegg.com",
		"chatpdf.com",
		"01.ai",
		"alice.yandex.ru",
	}
	for _, d := range casi {
		if got := Classifica(d); got != TipoAI {
			t.Errorf("Classifica(%q) = %q, atteso %q", d, got, TipoAI)
		}
	}
}

// TestClassificaSistema verifica che il rumore di sistema (telemetria,
// ad tech, ecc.) sia classificato come TipoSistema.
func TestClassificaSistema(t *testing.T) {
	casi := []string{
		"watson.telemetry.microsoft.com",
		"incoming.telemetry.mozilla.org",
		"detectportal.firefox.com",
		"play.google.com",
		"mtalk.google.com",
		"signaler-pa.clients6.google.com",
		"bam.nr-data.net",
		"browser-intake-datadoghq.com",
		"www.googleadservices.com",
		"ocsp.digicert.com",
		"login.live.com",
		"cdn.example.io",
		"static.example.io",
		"accounts.google.com",
		"accounts.google.it",
	}
	for _, d := range casi {
		if got := Classifica(d); got != TipoSistema {
			t.Errorf("Classifica(%q) = %q, atteso %q", d, got, TipoSistema)
		}
	}
}

// TestClassificaUtente verifica che i siti "veri" (non AI, non sistema) siano
// classificati come TipoUtente.
func TestClassificaUtente(t *testing.T) {
	casi := []string{
		"www.google.com",
		"google.com",
		"www.youtube.com",
		"www.facebook.com",
		"classroom.google.com",
		"mail.google.com",
		"drive.google.com",
		"www.maxplanck.edu.it",
		"www.eclipse.org",
		"www.bing.com",
		"www.canva.com",
	}
	for _, d := range casi {
		if got := Classifica(d); got != TipoUtente {
			t.Errorf("Classifica(%q) = %q, atteso %q", d, got, TipoUtente)
		}
	}
}

// TestClassificaPriorita verifica che AI vinca sui pattern sistema:
// es. "chat.openai.com" matcha sia "openai.com" (AI) sia "chat." (sistema)
// e deve essere classificato come AI.
func TestClassificaPriorita(t *testing.T) {
	casi := map[string]Tipo{
		"chat.openai.com":   TipoAI,      // matcha "openai.com" (AI) e "chat." (sistema) -> AI vince
		"chatpdf.com":       TipoAI,      // pattern AI esplicito
		"static.openai.com": TipoAI,      // matcha "openai.com" (AI) e "static." (sistema) -> AI vince
	}
	for d, atteso := range casi {
		if got := Classifica(d); got != atteso {
			t.Errorf("Classifica(%q) = %q, atteso %q", d, got, atteso)
		}
	}
}

// TestClassificaCaseInsensitive verifica che il match sia case-insensitive.
func TestClassificaCaseInsensitive(t *testing.T) {
	casi := map[string]Tipo{
		"CHAT.OPENAI.COM":   TipoAI,
		"Chat.OpenAI.com":   TipoAI,
		"WWW.GOOGLE.COM":    TipoUtente,
		"Watson.TELEMETRY.Microsoft.com": TipoSistema,
	}
	for d, atteso := range casi {
		if got := Classifica(d); got != atteso {
			t.Errorf("Classifica(%q) = %q, atteso %q", d, got, atteso)
		}
	}
}

// TestListeNonVuote sanity check: le liste hardcoded non sono accidentalmente vuote.
func TestListeNonVuote(t *testing.T) {
	if len(AIDomains()) < 50 {
		t.Errorf("AIDomains sembra troncato: %d elementi (atteso >= 50)", len(AIDomains()))
	}
	if len(PatternSistema) < 100 {
		t.Errorf("PatternSistema sembra troncato: %d elementi (atteso >= 100)", len(PatternSistema))
	}
}

// BenchmarkClassifica misura il throughput della classificazione.
// Caso peggiore: dominio che non matcha nulla (full scan delle due liste).
func BenchmarkClassifica(b *testing.B) {
	domini := []string{
		"chat.openai.com",                 // hit AI presto
		"watson.telemetry.microsoft.com",  // hit sistema
		"www.example.io",                  // miss totale (caso peggiore)
		"chatgpt.com",
		"www.youtube.com",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Classifica(domini[i%len(domini)])
	}
}
