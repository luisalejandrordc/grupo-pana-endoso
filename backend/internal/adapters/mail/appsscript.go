package mail

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// AppsScriptConfig configures the sender that delivers email through a Google Apps Script web app.
type AppsScriptConfig struct {
	// URL is the web app's address ("https://script.google.com/macros/s/.../exec").
	URL string
	// Secret is the shared HMAC secret. The script reads the same value from its MAIL_APPS_SCRIPT_HMAC_SECRET
	// script property.
	Secret string
	// Timeout bounds one delivery, including the redirect Apps Script answers with. Zero means 30 seconds.
	Timeout time.Duration
	// Client and the two functions below exist for tests; nil means the defaults.
	Client *http.Client
	Now    func() time.Time
	Nonce  func() (string, error)
}

type appsScript struct {
	url    string
	secret []byte
	client *http.Client
	now    func() time.Time
	nonce  func() (string, error)
}

// NewAppsScript returns a Sender that posts signed requests to the Apps Script web app.
//
// Every request is {timestamp, nonce, payload, signature}, where payload is the message as a JSON string and
// signature is hex(HMAC-SHA256(secret, timestamp + "\n" + nonce + "\n" + payload)). The script rejects stale
// timestamps (over five minutes) and repeated nonces, so a captured request cannot be replayed. A web app
// answers a POST with a redirect to the page that holds its output; the HTTP client follows it, which does not
// run the script a second time.
func NewAppsScript(c AppsScriptConfig) Sender {
	client := c.Client
	if client == nil {
		timeout := c.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	now := c.Now
	if now == nil {
		now = time.Now
	}
	nonce := c.Nonce
	if nonce == nil {
		nonce = randomNonce
	}
	return &appsScript{url: c.URL, secret: []byte(c.Secret), client: client, now: now, nonce: nonce}
}

func randomNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil // 32 hex characters, which is what the script requires
}

// scriptMessage is the payload the script reads.
type scriptMessage struct {
	To       string `json:"to"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	HTMLBody string `json:"htmlBody,omitempty"`
	ReplyTo  string `json:"replyTo,omitempty"`
	EventID  string `json:"eventId,omitempty"`
}

type scriptRequest struct {
	Timestamp int64  `json:"timestamp"`
	Nonce     string `json:"nonce"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

type scriptResponse struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error"`
	EventID string `json:"eventId"`
}

// sign returns the signature the script expects.
func sign(secret []byte, timestamp int64, nonce, payload string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strconv.FormatInt(timestamp, 10) + "\n" + nonce + "\n" + payload))
	return hex.EncodeToString(mac.Sum(nil))
}

const maxResponseBytes = 64 << 10

func (s *appsScript) Send(ctx context.Context, msg Message) error {
	payload, err := json.Marshal(scriptMessage{
		To: msg.To, Subject: msg.Subject, Body: msg.Body, HTMLBody: msg.HTMLBody, ReplyTo: msg.ReplyTo, EventID: msg.EventID,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRejected, err)
	}
	nonce, err := s.nonce()
	if err != nil {
		return fmt.Errorf("%w: could not generate a nonce: %v", ErrTransient, err)
	}
	timestamp := s.now().Unix()
	body, err := json.Marshal(scriptRequest{
		Timestamp: timestamp, Nonce: nonce, Payload: string(payload), Signature: sign(s.secret, timestamp, nonce, string(payload)),
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRejected, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTransient, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		// The error text of net/http includes the URL, which holds the deployment id: keep only the cause.
		return fmt.Errorf("%w: the request failed: %s", ErrTransient, redactURL(err, s.url))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("%w: could not read the answer: %v", ErrTransient, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: the email service answered HTTP %d", ErrTransient, resp.StatusCode)
	}
	var out scriptResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		// A web app that is not deployed for "anyone" answers with an HTML sign-in page instead of JSON.
		return fmt.Errorf("%w: the email service did not answer with JSON (check that the web app is deployed to run as its owner and open to anyone)", ErrTransient)
	}
	if out.OK {
		return nil
	}
	return scriptError(out.Error)
}

// scriptError maps the error codes of the Apps Script to the errors callers test for.
func scriptError(code string) error {
	switch code {
	case "EMAIL_QUOTA_EXCEEDED":
		return fmt.Errorf("%w (%s)", ErrQuotaExceeded, code)
	case "UNAUTHORIZED":
		return fmt.Errorf("%w (%s)", ErrUnauthorized, code)
	case "INVALID_MESSAGE", "INVALID_RECIPIENT":
		return fmt.Errorf("%w (%s)", ErrRejected, code)
	case "MAIL_SEND_FAILED":
		return fmt.Errorf("%w (%s)", ErrTransient, code)
	case "":
		return fmt.Errorf("%w: the email service answered without a result", ErrTransient)
	}
	return fmt.Errorf("%w: unknown error code %q", ErrTransient, code)
}

func redactURL(err error, url string) string {
	var msg string
	if err != nil {
		msg = err.Error()
	}
	for i := 0; i+len(url) <= len(msg); i++ {
		if msg[i:i+len(url)] == url {
			msg = msg[:i] + "<email service>" + msg[i+len(url):]
			break
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timed out"
	}
	return msg
}
