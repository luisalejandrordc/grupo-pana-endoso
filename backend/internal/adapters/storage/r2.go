package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// R2Config holds the Cloudflare R2 settings. The bucket must be readable at PublicBaseURL (a custom
// domain or the r2.dev address); only catalog images use it, and they are public by decision.
type R2Config struct {
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	PublicBaseURL   string
	Endpoint        string       // https://<account>.r2.cloudflarestorage.com
	HTTPClient      *http.Client // nil means a client with a 30 second timeout
	Now             func() time.Time
}

// R2 talks to Cloudflare R2 through its S3-compatible API, signing each request with AWS Signature
// Version 4. It needs no SDK.
type R2 struct {
	cfg R2Config
	now func() time.Time
}

// NewR2 builds the store.
func NewR2(cfg R2Config) *R2 {
	cfg.Endpoint = "https://" + cfg.AccountID + ".r2.cloudflarestorage.com"
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")
	cfg.PublicBaseURL = strings.TrimRight(cfg.PublicBaseURL, "/")
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &R2{cfg: cfg, now: now}
}

func (r *R2) PublicURL(key string) string { return r.cfg.PublicBaseURL + "/" + key }

// SignedURL returns a presigned GET address (AWS Signature Version 4 in the query string) that expires after
// ttl and makes the browser save the object as fileName. R2 allows at most seven days.
func (r *R2) SignedURL(_ context.Context, key, fileName string, ttl time.Duration) (string, error) {
	if ttl <= 0 || ttl > 7*24*time.Hour {
		return "", fmt.Errorf("r2 signed url: lifetime %s is not allowed", ttl)
	}
	u, err := url.Parse(r.cfg.Endpoint + escapePath("/"+r.cfg.Bucket+"/"+key))
	if err != nil {
		return "", err
	}
	query := u.Query()
	query.Set("response-content-disposition", contentDisposition(fileName))
	u.RawQuery = query.Encode()
	return presignV4(u, r.cfg.AccessKeyID, r.cfg.SecretAccessKey, "auto", "s3", r.now(), ttl), nil
}

// contentDisposition makes a download header that keeps accents in the file name and cannot be broken out of.
func contentDisposition(fileName string) string {
	ascii := make([]rune, 0, len(fileName))
	for _, c := range fileName {
		switch {
		case c < 0x20 || c == 0x7f || c == '"' || c == '\\' || c == ';' || c == '%':
			ascii = append(ascii, '_')
		case c > 0x7e:
			ascii = append(ascii, '_')
		default:
			ascii = append(ascii, c)
		}
	}
	return `attachment; filename="` + string(ascii) + `"; filename*=UTF-8''` + awsEscape(fileName)
}

// presignV4 returns u with the X-Amz-* query parameters and signature of a GET that is valid for ttl.
// Only the host header is signed and the payload is not, as S3 expects for a presigned download.
func presignV4(u *url.URL, accessKey, secretKey, region, service string, t time.Time, ttl time.Duration) string {
	t = t.UTC()
	amzDate := t.Format("20060102T150405Z")
	day := t.Format("20060102")
	scope := day + "/" + region + "/" + service + "/aws4_request"

	signed := *u
	query := signed.Query()
	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", accessKey+"/"+scope)
	query.Set("X-Amz-Date", amzDate)
	query.Set("X-Amz-Expires", fmt.Sprintf("%d", int64(ttl.Seconds())))
	query.Set("X-Amz-SignedHeaders", "host")
	signed.RawQuery = query.Encode()

	canonicalRequest := strings.Join([]string{
		http.MethodGet,
		signed.EscapedPath(),
		canonicalQuery(&signed),
		"host:" + signed.Host + "\n",
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")
	hashed := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", amzDate, scope, hex.EncodeToString(hashed[:])}, "\n")

	key := hmacSHA256([]byte("AWS4"+secretKey), day)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, service)
	key = hmacSHA256(key, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, stringToSign))

	// The signature goes last, after the sorted parameters it covers.
	signed.RawQuery = query.Encode() + "&X-Amz-Signature=" + signature
	return signed.String()
}

func (r *R2) Put(ctx context.Context, key, contentType string, data []byte) error {
	res, err := r.do(ctx, http.MethodPut, key, contentType, data)
	if err != nil {
		return err
	}
	return expect(res, "put", http.StatusOK)
}

func (r *R2) Delete(ctx context.Context, key string) error {
	res, err := r.do(ctx, http.MethodDelete, key, "", nil)
	if err != nil {
		return err
	}
	// S3 answers 204 for a delete; a missing object is not an error either way.
	if res.StatusCode == http.StatusNotFound {
		_ = res.Body.Close()
		return nil
	}
	return expect(res, "delete", http.StatusOK, http.StatusNoContent)
}

func expect(res *http.Response, what string, ok ...int) error {
	defer res.Body.Close()
	for _, code := range ok {
		if res.StatusCode == code {
			return nil
		}
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	return fmt.Errorf("r2 %s: status %d: %s", what, res.StatusCode, strings.TrimSpace(string(body)))
}

func (r *R2) do(ctx context.Context, method, key, contentType string, data []byte) (*http.Response, error) {
	objectPath := "/" + r.cfg.Bucket + "/" + key
	req, err := http.NewRequestWithContext(ctx, method, r.cfg.Endpoint+escapePath(objectPath), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	sum := sha256.Sum256(data)
	signV4(req, hex.EncodeToString(sum[:]), r.cfg.AccessKeyID, r.cfg.SecretAccessKey, "auto", "s3", r.now())
	res, err := r.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("r2 request: %w", err) // never include the request: it carries credentials
	}
	return res, nil
}

// escapePath percent-encodes each path segment the way S3 expects (a space is %20, a slash stays).
func escapePath(p string) string {
	segments := strings.Split(p, "/")
	for i, s := range segments {
		segments[i] = strings.ReplaceAll(url.PathEscape(s), "+", "%2B")
	}
	return strings.Join(segments, "/")
}

// signV4 adds the x-amz headers and the Authorization header (AWS Signature Version 4). It signs every
// header the request already has plus host, x-amz-content-sha256 and x-amz-date.
func signV4(req *http.Request, payloadHash, accessKey, secretKey, region, service string, t time.Time) {
	t = t.UTC()
	amzDate := t.Format("20060102T150405Z")
	day := t.Format("20060102")
	req.Header.Set("x-amz-content-sha256", payloadHash)
	req.Header.Set("x-amz-date", amzDate)

	headers := map[string]string{"host": req.URL.Host}
	for name, values := range req.Header {
		headers[strings.ToLower(name)] = strings.TrimSpace(strings.Join(values, ","))
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name + ":" + headers[name] + "\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		canonicalQuery(req.URL),
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := day + "/" + region + "/" + service + "/aws4_request"
	hashed := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", amzDate, scope, hex.EncodeToString(hashed[:])}, "\n")

	key := hmacSHA256([]byte("AWS4"+secretKey), day)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, service)
	key = hmacSHA256(key, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, stringToSign))

	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, scope, signedHeaders, signature))
}

func canonicalQuery(u *url.URL) string {
	query := u.Query()
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		values := query[k]
		sort.Strings(values)
		for _, v := range values {
			parts = append(parts, awsEscape(k)+"="+awsEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

func awsEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}
