// Runs Code.gs unchanged inside a small emulation of the Google services it uses, behind an HTTP server that
// answers like an Apps Script web app does: a POST is answered with a 302 to the page holding the output.
// It exists so the Go tests can prove that the backend's signature and payload are what the real script accepts.
// Usage: SECRET=... QUOTA=100 node harness.js   (prints "listening <port>")
const vm = require("vm");
const fs = require("fs");
const http = require("http");
const crypto = require("crypto");
const path = require("path");

const code = fs.readFileSync(path.join(__dirname, "..", "Code.gs"), "utf8");
const sent = [];
let quota = Number(process.env.QUOTA || 100);
const props = process.env.SECRET
  ? { MAIL_APPS_SCRIPT_HMAC_SECRET: process.env.SECRET }
  : {};
const cache = new Map();
const outputs = new Map();
let n = 0;

const sandbox = {
  console: { error() {}, log() {} },
  ContentService: {
    createTextOutput: (text) => ({
      text,
      setMimeType() {
        return this;
      },
    }),
    MimeType: { JSON: "json" },
  },
  MailApp: {
    getRemainingDailyQuota: () => quota,
    sendEmail: (o) => {
      if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(o.to))
        throw new Error("Invalid email: " + o.to);
      if (process.env.FAIL_SEND)
        throw new Error("Service invoked too many times");
      sent.push(o);
      quota--;
    },
  },
  PropertiesService: {
    getScriptProperties: () => ({ getProperty: (k) => props[k] || null }),
  },
  Utilities: {
    // Java returns signed bytes; the script's toHex copes with that, so the emulation does the same.
    computeHmacSha256Signature: (input, secret) =>
      Array.from(
        crypto.createHmac("sha256", secret).update(input, "utf8").digest(),
      ).map((b) => (b > 127 ? b - 256 : b)),
    Charset: { UTF_8: "UTF_8" },
  },
  LockService: {
    getScriptLock: () => ({
      waitLock() {},
      releaseLock() {},
      hasLock: () => true,
    }),
  },
  CacheService: {
    getScriptCache: () => ({
      get: (k) => cache.get(k) || null,
      put: (k, v) => cache.set(k, v),
    }),
  },
};
vm.createContext(sandbox);
vm.runInContext(code, sandbox);

const server = http.createServer((req, res) => {
  const chunks = [];
  req.on("data", (c) => chunks.push(c));
  req.on("end", () => {
    if (req.method === "POST" && req.url === "/exec") {
      const out = sandbox.doPost({
        postData: { contents: Buffer.concat(chunks).toString("utf8") },
      });
      const id = String(++n);
      outputs.set(id, out.text);
      res.writeHead(302, { Location: "/out/" + id });
      return res.end();
    }
    if (req.method === "GET" && req.url.startsWith("/out/")) {
      res.writeHead(200, { "Content-Type": "application/json" });
      return res.end(outputs.get(req.url.slice(5)) || "{}");
    }
    if (req.method === "GET" && req.url === "/sent") {
      res.writeHead(200, { "Content-Type": "application/json" });
      return res.end(JSON.stringify(sent));
    }
    res.writeHead(404);
    res.end();
  });
});
server.listen(0, "127.0.0.1", () =>
  console.log("listening " + server.address().port),
);
