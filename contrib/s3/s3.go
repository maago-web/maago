// Package s3 keeps a Maago project's media in a bucket of Amazon S3 or
// of a service that speaks its API (Cloudflare R2, MinIO), so that every
// instance of the site sees the same files: set maago.Config.Media to a
// Store. It signs its requests itself (AWS Signature Version 4) and uses
// the standard library only.
package s3

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/maago-web/maago"
)

// Store keeps media in Bucket, a bucket of the site's own. Its methods
// are maago.MediaStore's; the app calls maago's media functions.
// Each call checks the fields first, so one left empty fails the first
// save or read, naming it. A bucket of none fails Put and Delete naming
// NoSuchBucket, while Open finds no file in it.
type Store struct {
	Endpoint        string // the service's address, no bucket in it: https://s3.us-east-1.amazonaws.com, https://ACCOUNT.r2.cloudflarestorage.com, http://127.0.0.1:9000
	Region          string // us-east-1; auto for R2
	Bucket          string // the bucket's name
	AccessKeyID     string
	SecretAccessKey string
}

var _ maago.MediaStore = Store{}

// emptyHash is the SHA-256 of no bytes, a request with no body's.
const emptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// Put spools r to a temporary file to count and hash it, then sends it
// in one request: whole or not at all. One file is at most 5 GB.
func (s Store) Put(ctx context.Context, key, typ string, r io.Reader) (int64, error) {
	if err := s.checkName(key); err != nil {
		return 0, err
	}
	f, err := os.CreateTemp("", "maago-s3-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), r)
	if err != nil {
		return 0, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	header := http.Header{}
	if typ != "" {
		header.Set("Content-Type", typ)
	}
	resp, err := s.do(ctx, "PUT", key, nil, header, f, n, hex.EncodeToString(h.Sum(nil)))
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return n, nil
}

// Open asks the size and time of key's file; reading fetches from where
// the file is read, so a Range costs only its bytes.
func (s Store) Open(ctx context.Context, key string) (maago.MediaFile, error) {
	if err := s.checkName(key); err != nil {
		return nil, err
	}
	resp, err := s.do(ctx, "HEAD", key, nil, nil, nil, 0, emptyHash)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	size, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if err != nil || size < 0 {
		return nil, fmt.Errorf("s3: HEAD %s/%s: Content-Length %q is not a size", s.Bucket, key, resp.Header.Get("Content-Length"))
	}
	mod, err := http.ParseTime(resp.Header.Get("Last-Modified"))
	if err != nil {
		return nil, fmt.Errorf("s3: HEAD %s/%s: Last-Modified %q is not a time", s.Bucket, key, resp.Header.Get("Last-Modified"))
	}
	return &object{s: s, ctx: ctx, info: info{key, size, mod}}, nil
}

func (s Store) Delete(ctx context.Context, name string) error {
	if err := s.checkName(name); err != nil {
		return err
	}
	resp, err := s.do(ctx, "DELETE", name, nil, nil, nil, 0, emptyHash)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// List gives a page of at most 1,000 names, the bucket's. A name with a /
// in it, of a file put there another way, is listed but not deleted.
func (s Store) List(ctx context.Context, after string) ([]fs.FileInfo, error) {
	if _, err := s.check(); err != nil {
		return nil, err
	}
	q := url.Values{"list-type": {"2"}}
	if after != "" {
		q.Set("start-after", after)
	}
	resp, err := s.do(ctx, "GET", "", q, nil, nil, 0, emptyHash)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Contents []struct {
			Key          string
			Size         int64
			LastModified string
		}
	}
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("s3: listing %s: %w", s.Bucket, err)
	}
	var infos []fs.FileInfo
	for _, c := range result.Contents {
		mod, err := time.Parse(time.RFC3339, c.LastModified)
		if err != nil {
			return nil, fmt.Errorf("s3: listing %s: %s's LastModified %q is not a time", s.Bucket, c.Key, c.LastModified)
		}
		infos = append(infos, info{c.Key, c.Size, mod})
	}
	return infos, nil
}

// URL is a presigned GET of m's file that downloads it, under m.Name,
// with the type Put kept; ttl is whole seconds from one to 7 days.
func (s Store) URL(m maago.Media, ttl time.Duration) (string, error) {
	u, err := s.check()
	if err != nil {
		return "", err
	}
	if !isKey(m.Key) {
		return "", fmt.Errorf("s3: media key %q is not one maago.SaveMedia makes", m.Key)
	}
	if ttl < time.Second || ttl > 7*24*time.Hour || ttl%time.Second != 0 {
		return "", fmt.Errorf("s3: a URL lasts whole seconds from one to 7 days, not %v", ttl)
	}
	disposition := "attachment" // as maago.ServeMedia
	if m.Name != "" {
		disposition = mime.FormatMediaType("attachment", map[string]string{"filename": m.Name})
	}
	s.path(u, m.Key)
	return s.presign(u, time.Now(), ttl, url.Values{"response-content-disposition": {disposition}}), nil
}

// check checks the fields and returns Endpoint's URL.
func (s Store) check() (*url.URL, error) {
	for _, f := range [][2]string{{"Endpoint", s.Endpoint}, {"Region", s.Region}, {"Bucket", s.Bucket}, {"AccessKeyID", s.AccessKeyID}, {"SecretAccessKey", s.SecretAccessKey}} {
		if f[1] == "" {
			return nil, fmt.Errorf("s3: Store.%s is empty", f[0])
		}
	}
	const endpoint = "is the service's address, such as https://s3.us-east-1.amazonaws.com: http or https, a host, and no path; the bucket goes in Store.Bucket"
	u, err := url.Parse(s.Endpoint)
	if err != nil { // its text may hold a secret
		return nil, errors.New("s3: Store.Endpoint is no URL; it " + endpoint)
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return nil, fmt.Errorf("s3: Store.Endpoint %q %s", u.Redacted(), endpoint)
	}
	if !validBucket(s.Bucket) {
		return nil, fmt.Errorf("s3: Store.Bucket %q is not a bucket's name: 3 to 63 of a-z, 0-9, . and -, a letter or digit first and last", s.Bucket)
	}
	return u, nil
}

// checkName checks the fields, and that name is one file's, so that no
// request is of the bucket itself or of another path.
func (s Store) checkName(name string) error {
	if _, err := s.check(); err != nil {
		return err
	}
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("s3: %q is not the name of one file in the bucket", name)
	}
	return nil
}

func validBucket(b string) bool {
	if len(b) < 3 || len(b) > 63 || strings.HasPrefix(b, ".") || strings.HasPrefix(b, "-") || strings.HasSuffix(b, ".") || strings.HasSuffix(b, "-") {
		return false
	}
	for _, c := range b {
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '.' || c == '-') {
			return false
		}
	}
	return true
}

// isKey reports whether key is one maago.SaveMedia makes.
func isKey(key string) bool {
	if len(key) != 32 {
		return false
	}
	for _, c := range key {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}

// path sets u's path to name's in the bucket, the bucket's for "",
// escaped as signed.
func (s Store) path(u *url.URL, name string) {
	u.Path, u.RawPath = "/"+s.Bucket, "/"+escape(s.Bucket, false)
	if name != "" {
		u.Path += "/" + name
		u.RawPath += "/" + escape(name, false)
	}
}

// client follows no redirect: a PUT or DELETE sent on as a GET would end
// in a 200 for what was never done.
var client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// do sends a signed request of name's file, the bucket's for "", with
// body's n bytes whose SHA-256 is payload. A status but 2xx is an error;
// a 404 of a file's HEAD or GET is fs.ErrNotExist, of nothing else (a
// bucket of none).
func (s Store) do(ctx context.Context, method, name string, query url.Values, header http.Header, body io.Reader, n int64, payload string) (*http.Response, error) {
	u, err := s.check()
	if err != nil {
		return nil, err
	}
	s.path(u, name)
	u.RawQuery = canonicalQuery(query)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		req.Body, req.ContentLength = io.NopCloser(body), n
	} else {
		req.Body = http.NoBody // a body of unknown length would go chunked, which S3 refuses
	}
	for k, v := range header {
		req.Header[k] = v
	}
	s.sign(req, payload, time.Now())
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 == 2 {
		return resp, nil
	}
	defer resp.Body.Close()
	msg := resp.Status
	var e struct{ Code, Message string }
	if xml.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&e) == nil && e.Code != "" {
		msg += ": " + e.Code + ": " + e.Message
	}
	err = fmt.Errorf("s3: %s %s/%s: %s", method, s.Bucket, name, msg)
	if resp.StatusCode == http.StatusNotFound && name != "" && (method == "HEAD" || method == "GET") {
		err = fmt.Errorf("%w (%w)", err, fs.ErrNotExist)
	}
	return nil, err
}

// sign signs req by its header: the host and every header req has now,
// so Range and Content-Type are signed; what the transport adds later
// (User-Agent, Content-Length) is not.
func (s Store) sign(req *http.Request, payload string, now time.Time) {
	date := now.UTC().Format("20060102T150405Z")
	req.Header.Set("X-Amz-Date", date)
	req.Header.Set("X-Amz-Content-Sha256", payload)
	names := []string{"host"}
	values := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		k = strings.ToLower(k)
		names = append(names, k)
		values[k] = strings.TrimSpace(strings.Join(v, ","))
	}
	slices.Sort(names)
	var headers strings.Builder
	for _, k := range names {
		headers.WriteString(k + ":" + values[k] + "\n")
	}
	signed := strings.Join(names, ";")
	q, _ := url.ParseQuery(req.URL.RawQuery)
	sig := s.signature(date, req.Method, req.URL.EscapedPath(), canonicalQuery(q), headers.String(), signed, payload)
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.AccessKeyID+"/"+s.scope(date)+",SignedHeaders="+signed+",Signature="+sig)
}

// presign is u signed in its query for a GET of ttl, with q's values.
func (s Store) presign(u *url.URL, now time.Time, ttl time.Duration, q url.Values) string {
	date := now.UTC().Format("20060102T150405Z")
	if q == nil {
		q = url.Values{}
	}
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", s.AccessKeyID+"/"+s.scope(date))
	q.Set("X-Amz-Date", date)
	q.Set("X-Amz-Expires", strconv.FormatInt(int64(ttl/time.Second), 10))
	q.Set("X-Amz-SignedHeaders", "host")
	query := canonicalQuery(q)
	sig := s.signature(date, "GET", u.EscapedPath(), query, "host:"+u.Host+"\n", "host", "UNSIGNED-PAYLOAD")
	signed := *u
	signed.RawQuery = query + "&X-Amz-Signature=" + sig
	return signed.String()
}

func (s Store) scope(date string) string {
	return date[:8] + "/" + s.Region + "/s3/aws4_request"
}

// signature is the hex HMAC of a canonical request's parts.
func (s Store) signature(date, method, path, query, headers, signed, payload string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{method, path, query, headers, signed, payload}, "\n")))
	k := []byte("AWS4" + s.SecretAccessKey)
	for _, part := range []string{date[:8], s.Region, "s3", "aws4_request", "AWS4-HMAC-SHA256\n" + date + "\n" + s.scope(date) + "\n" + hex.EncodeToString(sum[:])} {
		mac := hmac.New(sha256.New, k)
		mac.Write([]byte(part))
		k = mac.Sum(nil)
	}
	return hex.EncodeToString(k)
}

// canonicalQuery is q escaped and sorted by key, then value, as signed
// and as sent: a space is %20, never +.
func canonicalQuery(q url.Values) string {
	var parts []string
	for k, vs := range q {
		for _, v := range vs {
			parts = append(parts, escape(k, false)+"="+escape(v, false))
		}
	}
	slices.Sort(parts)
	return strings.Join(parts, "&")
}

// escape is AWS's UriEncode: the unreserved bytes of RFC 3986 as they
// are, each other as %XX, and / too unless slash.
func escape(s string, slash bool) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~' || slash && c == '/' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// object is a file of the bucket, read by Range GETs from where it is
// read: one GET while it is read in order.
type object struct {
	s    Store
	ctx  context.Context
	info info
	off  int64         // where the next Read reads
	body io.ReadCloser // a GET's, from at
	at   int64
}

func (o *object) Read(p []byte) (int, error) {
	if o.off >= o.info.size {
		return 0, io.EOF
	}
	if o.body == nil || o.at != o.off {
		if o.body != nil {
			o.body.Close()
			o.body = nil
		}
		resp, err := o.s.do(o.ctx, "GET", o.info.name, nil, http.Header{"Range": {fmt.Sprintf("bytes=%d-", o.off)}}, nil, 0, emptyHash)
		if err != nil {
			return 0, err
		}
		if cr := resp.Header.Get("Content-Range"); resp.StatusCode != http.StatusPartialContent || !strings.HasPrefix(cr, fmt.Sprintf("bytes %d-", o.off)) {
			resp.Body.Close()
			return 0, fmt.Errorf("s3: GET %s/%s from byte %d: %s, Content-Range %q", o.s.Bucket, o.info.name, o.off, resp.Status, cr)
		}
		o.body, o.at = resp.Body, o.off
	}
	n, err := o.body.Read(p)
	o.off += int64(n)
	o.at = o.off
	if err == io.EOF && o.off < o.info.size {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

func (o *object) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += o.off
	case io.SeekEnd:
		offset += o.info.size
	default:
		return 0, fmt.Errorf("s3: seek: whence %d", whence)
	}
	if offset < 0 {
		return 0, fmt.Errorf("s3: seek to %d, before the start", offset)
	}
	o.off = offset
	return offset, nil
}

func (o *object) Stat() (fs.FileInfo, error) { return o.info, nil }

func (o *object) Close() error {
	if o.body == nil {
		return nil
	}
	return o.body.Close()
}

// info is a file's, as Open and List tell it.
type info struct {
	name string
	size int64
	mod  time.Time
}

func (i info) Name() string       { return i.name }
func (i info) Size() int64        { return i.size }
func (i info) Mode() fs.FileMode  { return 0o444 }
func (i info) ModTime() time.Time { return i.mod }
func (i info) IsDir() bool        { return false }
func (i info) Sys() any           { return nil }
