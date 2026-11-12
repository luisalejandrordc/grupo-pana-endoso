package mail

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testSecret = "0123456789abcdef0123456789abcdef-secret"

func msg() Message {
	return Message{To: "ana@example.com", Subject: "Hola", Body: "Texto", EventID: "evt-1"}
}

// startScript runs the real Code.gs under the node harness and returns its base URL and a way to read what it
// "sent". The test is skipped where node is not installed.
func startScript(t *testing.T, env ...string) (url string, sent func() []map[string]any) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed: the Apps Script compatibility test needs it")
	}
	harness, err := filepath.Abs("../../../apps-script/test/harness.js")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", harness)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("the harness did not start: %v", err)
	}
	var port int
	if _, err := fmt.Sscanf(line, "listening %d", &port); err != nil {
		t.Fatalf("unexpected harness output %q", line)
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	return base + "/exec", func() []map[string]any {
		resp, err := http.Get(base + "/sent")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var list []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			t.Fatal(err)
		}
		return list
	}
}

func TestTheRealScriptAcceptsOurRequests(t *testing.T) {
	url, sent := startScript(t, "SECRET="+testSecret)
	s := NewAppsScript(AppsScriptConfig{URL: url, Secret: testSecret})

	m := msg()
	m.HTMLBody, m.ReplyTo = "<p>Texto</p>", "asesor@grupopana.pe"
	if err := s.Send(context.Background(), m); err != nil {
		t.Fatalf("the script must accept a correctly signed request: %v", err)
	}
	got := sent()
	if len(got) != 1 || got[0]["to"] != "ana@example.com" || got[0]["subject"] != "Hola" || got[0]["htmlBody"] != "<p>Texto</p>" ||
		got[0]["replyTo"] != "asesor@grupopana.pe" || got[0]["name"] != "EndosoFlow | Grupo Pana" {
		t.Fatalf("what the script sent: %v", got)
	}
}

func TestTheRealScriptRejectsABadSignatureWithUnauthorized(t *testing.T) {
	url, sent := startScript(t, "SECRET="+testSecret)
	s := NewAppsScript(AppsScriptConfig{URL: url, Secret: "another-secret-another-secret-another"})
	err := s.Send(context.Background(), msg())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
	if len(sent()) != 0 {
		t.Fatal("nothing may be sent for an unauthorized request")
	}
}

func TestTheRealScriptRejectsAStaleTimestamp(t *testing.T) {
	url, _ := startScript(t, "SECRET="+testSecret)
	s := NewAppsScript(AppsScriptConfig{URL: url, Secret: testSecret, Now: func() time.Time { return time.Now().Add(-10 * time.Minute) }})
	if err := s.Send(context.Background(), msg()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("a request older than five minutes must be refused: %v", err)
	}
}

func TestTheRealScriptRefusesAReplayedNonce(t *testing.T) {
	url, sent := startScript(t, "SECRET="+testSecret)
	s := NewAppsScript(AppsScriptConfig{URL: url, Secret: testSecret, Nonce: func() (string, error) { return strings.Repeat("ab", 16), nil }})
	if err := s.Send(context.Background(), msg()); err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), msg()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("a repeated nonce is a replay: %v", err)
	}
	if len(sent()) != 1 {
		t.Fatal("the replayed request must not send a second email")
	}
}

func TestTheRealScriptReportsAnExhaustedQuota(t *testing.T) {
	url, _ := startScript(t, "SECRET="+testSecret, "QUOTA=1")
	s := NewAppsScript(AppsScriptConfig{URL: url, Secret: testSecret})
	if err := s.Send(context.Background(), msg()); err != nil {
		t.Fatal(err)
	}
	err := s.Send(context.Background(), msg())
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("the 101st email of the day must come back as ErrQuotaExceeded: %v", err)
	}
	if errors.Is(err, ErrTransient) || errors.Is(err, ErrRejected) {
		t.Fatal("a quota error must not look like any other kind of failure")
	}
}

func TestTheRealScriptClassifiesRecipientsAndFailures(t *testing.T) {
	url, _ := startScript(t, "SECRET="+testSecret)
	s := NewAppsScript(AppsScriptConfig{URL: url, Secret: testSecret})

	bad := msg()
	bad.To = "not-an-address"
	if err := s.Send(context.Background(), bad); !errors.Is(err, ErrRejected) {
		t.Fatalf("an address Google refuses is a permanent failure: %v", err)
	}
	empty := msg()
	empty.Body = ""
	if err := s.Send(context.Background(), empty); !errors.Is(err, ErrRejected) {
		t.Fatalf("a message without a body is rejected: %v", err)
	}

	url, _ = startScript(t, "SECRET="+testSecret, "FAIL_SEND=1")
	s = NewAppsScript(AppsScriptConfig{URL: url, Secret: testSecret})
	if err := s.Send(context.Background(), msg()); !errors.Is(err, ErrTransient) {
		t.Fatalf("a failure inside Google is transient: %v", err)
	}
}

func TestTheRealScriptWithoutASecretRefusesEverything(t *testing.T) {
	url, _ := startScript(t) // no SECRET property
	s := NewAppsScript(AppsScriptConfig{URL: url, Secret: testSecret})
	if err := s.Send(context.Background(), msg()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("a script without its secret property must refuse: %v", err)
	}
}

// --- behavior that does not need the script ---

func TestSignatureMatchesTheDocumentedFormat(t *testing.T) {
	// hex(HMAC-SHA256("key", "1700000000\nabc\n{}")), computed independently of this package.
	got := sign([]byte("key"), 1700000000, "abc", "{}")
	if len(got) != 64 || strings.ToLower(got) != got {
		t.Fatalf("signature must be 64 lower-case hex characters: %q", got)
	}
	if got == sign([]byte("key"), 1700000001, "abc", "{}") || got == sign([]byte("key"), 1700000000, "abd", "{}") || got == sign([]byte("key"), 1700000000, "abc", "{ }") {
		t.Fatal("the timestamp, the nonce and the payload must all be part of the signature")
	}
}

func TestAnAnswerThatIsNotJSONIsAdvice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "<html>Sign in to continue</html>")
	}))
	defer server.Close()
	err := NewAppsScript(AppsScriptConfig{URL: server.URL, Secret: testSecret}).Send(context.Background(), msg())
	if !errors.Is(err, ErrTransient) || !strings.Contains(err.Error(), "open to anyone") {
		t.Fatalf("a sign-in page means a wrong deployment, and the error must say so: %v", err)
	}
}

func TestHTTPFailuresAndTimeoutsAreTransientAndHideTheURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	err := NewAppsScript(AppsScriptConfig{URL: server.URL, Secret: testSecret}).Send(context.Background(), msg())
	server.Close()
	if !errors.Is(err, ErrTransient) {
		t.Fatalf("HTTP 502: %v", err)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(300 * time.Millisecond) }))
	defer slow.Close()
	err = NewAppsScript(AppsScriptConfig{URL: slow.URL + "/macros/s/DEPLOYMENT-ID/exec", Secret: testSecret, Timeout: 50 * time.Millisecond}).Send(context.Background(), msg())
	if !errors.Is(err, ErrTransient) {
		t.Fatalf("timeout: %v", err)
	}
	if strings.Contains(err.Error(), "DEPLOYMENT-ID") || strings.Contains(err.Error(), testSecret) {
		t.Fatalf("the error must not leak the deployment id or the secret: %v", err)
	}
}

func TestUnknownErrorCodesAreTransient(t *testing.T) {
	if !errors.Is(scriptError("SOMETHING_NEW"), ErrTransient) || !errors.Is(scriptError(""), ErrTransient) {
		t.Fatal("an unknown code must be retried, not dropped")
	}
}

func TestAllowListAcceptsAddressesAndDomainsIgnoringCase(t *testing.T) {
	l := NewAllowList([]string{" Ana@Example.com ", "@grupopana.pe", ""})
	for to, want := range map[string]bool{
		"ana@example.com": true, "ANA@EXAMPLE.COM": true, "luis@grupopana.pe": true, "luis@GrupoPana.pe": true,
		"luis@evil-grupopana.pe": false, "luis@sub.grupopana.pe": false, "bob@example.com": false, "": false, "@grupopana.pe": false,
	} {
		if l.Allows(to) != want {
			t.Errorf("Allows(%q) = %v, want %v", to, !want, want)
		}
	}
	if NewAllowList(nil).Allows("ana@example.com") {
		t.Fatal("an empty list allows nobody")
	}
}

func TestRestrictRefusesWithoutCallingTheSender(t *testing.T) {
	calls := 0
	inner := senderFunc(func(context.Context, Message) error { calls++; return nil })
	s := Restrict(inner, NewAllowList([]string{"@grupopana.pe"}))
	if err := s.Send(context.Background(), Message{To: "cliente@gmail.com"}); !errors.Is(err, ErrRecipientNotAllowed) || calls != 0 {
		t.Fatalf("a recipient outside the list: err=%v calls=%d", err, calls)
	}
	if err := s.Send(context.Background(), Message{To: "asesor@grupopana.pe"}); err != nil || calls != 1 {
		t.Fatalf("a recipient on the list: err=%v calls=%d", err, calls)
	}
}

type senderFunc func(context.Context, Message) error

func (f senderFunc) Send(ctx context.Context, m Message) error { return f(ctx, m) }
