# Email delivery with Google Apps Script

The Go backend decides what to send and to whom (it renders every email and keeps the queue in
`notification_logs`). `Code.gs` only delivers: it verifies a signed request and calls `MailApp.sendEmail`
from the project's Gmail account.

## Deploy

1. Sign in with the project's Gmail account, create a project at script.google.com and paste `Code.gs`.
2. Project Settings → Script properties → add `MAIL_APPS_SCRIPT_HMAC_SECRET` with a long random value
   (`openssl rand -hex 32`).
3. Deploy → New deployment → type **Web app**, _Execute as: Me_, _Who has access: Anyone_. The signature, not the
   link, is what authenticates a request. Authorize the mail permission when asked.
4. Copy the `/exec` address into the backend as `MAIL_APPS_SCRIPT_URL`, and the same secret as
   `MAIL_APPS_SCRIPT_HMAC_SECRET`. After editing the script, create a **new version** of the deployment: the
   address stays the same, but the old code keeps running until you do.

## Protocol

Request (POST, JSON): `{ timestamp, nonce, payload, signature }`

- `payload` is the message as a JSON string: `{ to, subject, body, htmlBody?, replyTo?, eventId? }`
- `signature = hex(HMAC-SHA256(secret, timestamp + "\n" + nonce + "\n" + payload))`
- The script refuses timestamps more than 5 minutes off (keep the server clock right) and any nonce it has seen
  in the last 5 minutes.

Response: `{ ok: true, eventId }` or `{ ok: false, error }`:

| error                                               | meaning                                                      | what the backend does                                                                                       |
| --------------------------------------------------- | ------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------- |
| `EMAIL_QUOTA_EXCEEDED`                              | the day's quota (100/day on a free Gmail account) is used up | leaves the notification pending, **does not count an attempt**, and stops sending for an hour               |
| `UNAUTHORIZED`                                      | wrong secret, stale timestamp or repeated nonce              | leaves it pending, does not count an attempt, pauses 10 minutes and logs an error (a configuration problem) |
| `INVALID_MESSAGE`, `INVALID_RECIPIENT`              | will never work                                              | marks the notification `failed`                                                                             |
| `MAIL_SEND_FAILED` (or anything else, or no answer) | transient                                                    | retries after 5 min, 15 min, 1 h, 3 h; `failed` after 5 attempts                                            |

`INVALID_RECIPIENT` is the one change made to the original script: an address Google refuses is reported as a
permanent error so the backend does not retry it.

## Test without Google

`test/harness.js` runs `Code.gs` unchanged inside a small emulation of Apps Script (including the redirect a web
app answers a POST with). The Go tests in `internal/adapters/mail` use it to prove the backend's signatures are
accepted by the real script, and that replays, stale timestamps, bad secrets and the quota behave as above.
They are skipped when `node` is not installed.

## Good to know

- The same account's quota is shared by **everything** the backend sends: activation and password-reset emails
  too. 100/day is plenty for a prototype; if the quota ever runs out, queued notifications simply wait.
- `MailApp` quotas for consumer Gmail accounts are lower than for Google Workspace accounts.
