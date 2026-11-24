// Package s3fake is an S3 server for tests: one bucket, media, in memory.
// It checks each request's AWS Signature Version 4 with code of its own,
// never contrib/s3's, so a mistake there is not shared here.
package s3fake

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maago-web/maago/contrib/s3"
)

const (
	bucket = "media"
	region = "us-east-1"
	id     = "AKIDFAKE"
	secret = "fake-secret"
)

// fakes are the servers New started, by endpoint, for Touch.
var fakes sync.Map

// New starts a server for the test and returns a Store of its bucket.
func New(t *testing.T) s3.Store {
	f := &fake{files: map[string]file{}}
	srv := httptest.NewServer(f)
	fakes.Store(srv.URL, f)
	t.Cleanup(func() {
		fakes.Delete(srv.URL)
		srv.Close()
	})
	return s3.Store{Endpoint: srv.URL, Region: region, Bucket: bucket, AccessKeyID: id, SecretAccessKey: secret}
}

// Touch gives the file of name in the bucket of s, a Store New returned,
// the time mod, made empty if there is none: an old file, or a name
// contrib/s3 never sends, such as a/b.
func Touch(s s3.Store, name string, mod time.Time) {
	v, _ := fakes.Load(s.Endpoint)
	f := v.(*fake)
	f.mu.Lock()
	defer f.mu.Unlock()
	file := f.files[name]
	file.mod = mod
	f.files[name] = file
}

type fake struct {
	mu    sync.Mutex
	files map[string]file
}

type file struct {
	data []byte
	typ  string
	mod  time.Time
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		fail(w, 400, "IncompleteBody", err.Error())
		return
	}
	if code, name, msg := verify(r, body); code != 0 {
		fail(w, code, name, msg)
		return
	}
	b, key, _ := strings.Cut(strings.TrimPrefix(r.URL.EscapedPath(), "/"), "/")
	if b != bucket {
		fail(w, 404, "NoSuchBucket", "The specified bucket does not exist")
		return
	}
	key, err = url.PathUnescape(key)
	if err != nil {
		fail(w, 400, "InvalidURI", err.Error())
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case key == "" && r.Method == "GET" && r.URL.Query().Get("list-type") == "2":
		f.list(w, r.URL.Query().Get("start-after"))
	case key == "":
		fail(w, 400, "NotImplemented", "only ListObjectsV2 on the bucket")
	case r.Method == "PUT":
		if r.ContentLength < 0 || slices.Contains(r.TransferEncoding, "chunked") {
			fail(w, 411, "MissingContentLength", "You must provide the Content-Length HTTP header")
			return
		}
		f.files[key] = file{body, r.Header.Get("Content-Type"), time.Now()}
	case r.Method == "GET" || r.Method == "HEAD":
		file, ok := f.files[key]
		if !ok {
			fail(w, 404, "NoSuchKey", "The specified key does not exist.")
			return
		}
		typ := file.typ
		if typ == "" {
			typ = "binary/octet-stream" // as S3
		}
		w.Header()["Content-Type"] = []string{typ} // never sniffed
		if d := r.URL.Query().Get("response-content-disposition"); d != "" && r.URL.Query().Has("X-Amz-Signature") {
			w.Header().Set("Content-Disposition", d)
		}
		http.ServeContent(w, r, "", file.mod, strings.NewReader(string(file.data)))
	case r.Method == "DELETE":
		delete(f.files, key)
		w.WriteHeader(http.StatusNoContent) // gone or never there
	default:
		fail(w, 400, "NotImplemented", r.Method)
	}
}

// list answers ListObjectsV2, two names a page so that pages are many.
func (f *fake) list(w http.ResponseWriter, after string) {
	type content struct {
		Key          string
		LastModified string
		Size         int
	}
	var result struct {
		XMLName     xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListBucketResult"`
		Name        string
		IsTruncated bool
		Contents    []content
	}
	result.Name = bucket
	var keys []string
	for k := range f.files {
		if k > after {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	if len(keys) > 2 {
		keys, result.IsTruncated = keys[:2], true
	}
	for _, k := range keys {
		result.Contents = append(result.Contents, content{k, f.files[k].mod.UTC().Format("2006-01-02T15:04:05.000Z"), len(f.files[k].data)})
	}
	w.Header().Set("Content-Type", "application/xml")
	xml.NewEncoder(w).Encode(result)
}

func fail(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	xml.NewEncoder(w).Encode(struct {
		XMLName xml.Name `xml:"Error"`
		Code    string
		Message string
	}{Code: code, Message: msg})
}

// verify checks r's signature, in its header or its query (a presigned
// URL), and returns the status, code and message of a refusal, 0 for none.
func verify(r *http.Request, body []byte) (int, string, string) {
	q := r.URL.Query()
	var date, credential, signedHeaders, signature, payload string
	if q.Has("X-Amz-Signature") {
		if q.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
			return 400, "AuthorizationQueryParametersError", "X-Amz-Algorithm"
		}
		date, credential, signedHeaders, signature = q.Get("X-Amz-Date"), q.Get("X-Amz-Credential"), q.Get("X-Amz-SignedHeaders"), q.Get("X-Amz-Signature")
		payload = "UNSIGNED-PAYLOAD"
		expires, err := strconv.Atoi(q.Get("X-Amz-Expires"))
		if err != nil || expires < 1 || expires > 604800 {
			return 400, "AuthorizationQueryParametersError", "X-Amz-Expires must be from 1 to 604800"
		}
		at, err := time.Parse("20060102T150405Z", date)
		if err != nil {
			return 400, "AuthorizationQueryParametersError", "X-Amz-Date"
		}
		if time.Now().After(at.Add(time.Duration(expires) * time.Second)) {
			return 403, "AccessDenied", "Request has expired"
		}
	} else {
		auth, ok := strings.CutPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ")
		if !ok {
			return 403, "AccessDenied", "no AWS4-HMAC-SHA256 Authorization"
		}
		for part := range strings.SplitSeq(auth, ",") {
			k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
			switch k {
			case "Credential":
				credential = v
			case "SignedHeaders":
				signedHeaders = v
			case "Signature":
				signature = v
			}
		}
		date, payload = r.Header.Get("X-Amz-Date"), r.Header.Get("X-Amz-Content-Sha256")
		if sum := sha256.Sum256(body); payload != hex.EncodeToString(sum[:]) {
			return 400, "XAmzContentSHA256Mismatch", "The provided 'x-amz-content-sha256' header does not match what was computed."
		}
	}
	if len(date) < 8 || credential != id+"/"+date[:8]+"/"+region+"/s3/aws4_request" {
		return 403, "InvalidAccessKeyId", "credential " + credential
	}

	// The canonical request, from what came over the wire.
	var query []string
	for k, vs := range q {
		if k == "X-Amz-Signature" {
			continue
		}
		for _, v := range vs {
			query = append(query, uriEncode(k)+"="+uriEncode(v))
		}
	}
	slices.Sort(query)
	var headers strings.Builder
	for h := range strings.SplitSeq(signedHeaders, ";") {
		v := strings.Join(r.Header.Values(h), ",")
		if h == "host" {
			v = r.Host
		}
		fmt.Fprintf(&headers, "%s:%s\n", h, strings.TrimSpace(v))
	}
	canonical := strings.Join([]string{r.Method, r.URL.EscapedPath(), strings.Join(query, "&"), headers.String(), signedHeaders, payload}, "\n")
	sum := sha256.Sum256([]byte(canonical))
	scope := date[:8] + "/" + region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + date + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	k := []byte("AWS4" + secret)
	for _, part := range []string{date[:8], region, "s3", "aws4_request", toSign} {
		mac := hmac.New(sha256.New, k)
		mac.Write([]byte(part))
		k = mac.Sum(nil)
	}
	if !hmac.Equal([]byte(hex.EncodeToString(k)), []byte(signature)) {
		return 403, "SignatureDoesNotMatch", "The request signature we calculated does not match the signature you provided."
	}
	return 0, "", ""
}

// uriEncode is AWS's UriEncode of a query's key or value.
func uriEncode(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
