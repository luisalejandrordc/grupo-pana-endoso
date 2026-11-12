/**
 * EndosoFlow | Grupo Pana: email delivery web app.
 *
 * The Go backend decides what to send and to whom; this script only delivers. Deploy it as a web app that
 * executes as you and is accessible to "Anyone" (the HMAC signature is the authentication), and store the
 * shared secret in Project Settings > Script properties as MAIL_APPS_SCRIPT_HMAC_SECRET (the same value
 * as the backend's MAIL_APPS_SCRIPT_HMAC_SECRET).
 *
 * Request:  { timestamp, nonce, payload, signature }
 *           payload   = JSON string of { to, subject, body, htmlBody?, replyTo?, eventId? }
 *           signature = hex(HMAC-SHA256(secret, timestamp + "\n" + nonce + "\n" + payload))
 * Response: { ok: true, eventId } or { ok: false, error }, with error one of
 *           UNAUTHORIZED, INVALID_MESSAGE, INVALID_RECIPIENT, EMAIL_QUOTA_EXCEEDED, MAIL_SEND_FAILED
 */
function doPost(e) {
  try {
    const request = JSON.parse(e.postData.contents);

    // Verify the request before sending anything.
    if (!isAuthorized(request)) {
      return jsonResponse({
        ok: false,
        error: "UNAUTHORIZED",
      });
    }

    const message = JSON.parse(request.payload);

    if (!message.to || !message.subject || !message.body) {
      return jsonResponse({
        ok: false,
        error: "INVALID_MESSAGE",
      });
    }

    // Check the remaining daily recipient quota.
    if (MailApp.getRemainingDailyQuota() < 1) {
      return jsonResponse({
        ok: false,
        error: "EMAIL_QUOTA_EXCEEDED",
      });
    }

    const options = {
      to: message.to,
      subject: message.subject,
      body: message.body,
      name: "EndosoFlow | Grupo Pana",
    };

    if (message.htmlBody) {
      options.htmlBody = message.htmlBody;
    }

    if (message.replyTo) {
      options.replyTo = message.replyTo;
    }

    MailApp.sendEmail(options);

    return jsonResponse({
      ok: true,
      eventId: message.eventId,
    });
  } catch (error) {
    console.error("Email service error: " + error.message);

    // ADDED: an address Google refuses will never work, so tell the backend not to retry it.
    if (/^Invalid email/i.test(String(error && error.message))) {
      return jsonResponse({
        ok: false,
        error: "INVALID_RECIPIENT",
      });
    }

    return jsonResponse({
      ok: false,
      error: "MAIL_SEND_FAILED",
    });
  }
}

function jsonResponse(data) {
  return ContentService.createTextOutput(JSON.stringify(data)).setMimeType(
    ContentService.MimeType.JSON,
  );
}

function isAuthorized(request) {
  const secret = PropertiesService.getScriptProperties().getProperty(
    "MAIL_APPS_SCRIPT_HMAC_SECRET",
  );

  if (!secret) return false;

  const timestamp = Number(request.timestamp);
  const nonce = String(request.nonce || "");
  const payload = String(request.payload || "");
  const signature = String(request.signature || "");

  // Reject requests older than five minutes.
  const now = Math.floor(Date.now() / 1000);

  if (!Number.isSafeInteger(timestamp) || Math.abs(now - timestamp) > 300) {
    return false;
  }

  if (!/^[a-f0-9]{32,}$/i.test(nonce)) {
    return false;
  }

  const signingInput = timestamp + "\n" + nonce + "\n" + payload;

  const expected = toHex(
    Utilities.computeHmacSha256Signature(
      signingInput,
      secret,
      Utilities.Charset.UTF_8,
    ),
  );

  if (!constantTimeEquals(expected, signature)) {
    return false;
  }

  // Basic replay protection using a short-lived cache.
  const lock = LockService.getScriptLock();

  try {
    lock.waitLock(5000);

    const cache = CacheService.getScriptCache();
    const key = "nonce:" + nonce;

    if (cache.get(key)) {
      return false;
    }

    cache.put(key, "1", 300);

    return true;
  } catch (error) {
    console.error("Request verification failed");
    return false;
  } finally {
    if (lock.hasLock()) {
      lock.releaseLock();
    }
  }
}

function toHex(bytes) {
  return bytes
    .map(function (b) {
      return ("0" + ((b + 256) % 256).toString(16)).slice(-2);
    })
    .join("");
}

function constantTimeEquals(a, b) {
  if (a.length !== b.length) return false;

  let result = 0;

  for (let i = 0; i < a.length; i++) {
    result |= a.charCodeAt(i) ^ b.charCodeAt(i);
  }

  return result === 0;
}
