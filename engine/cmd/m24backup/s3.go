package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	SIGN_ALGORITHM = "AWS4-HMAC-SHA256"
	SIGN_SERVICE   = "s3"
	SIGN_END       = "aws4_request"
	SIGN_DATE      = "20060102"
	SIGN_TIME      = "20060102T150405Z"
	HEADER_DATE    = "x-amz-date"
	HEADER_HASH    = "x-amz-content-sha256"
	EMPTY_HASH     = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	R2_REGION      = "auto"
	R2_DOMAIN      = ".r2.cloudflarestorage.com"
)

type r2Store struct {
	host, bucket, access, secret string
	client                       *http.Client
}

type object struct {
	Key  string
	Size int64
}

func hmacDigest(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func canonicalQuery(query url.Values) string {
	for _, values := range query {
		sort.Strings(values)
	}
	return strings.ReplaceAll(query.Encode(), "+", "%20")
}

// signRequest signs the exact escaped S3 path without cleaning or double-encoding it.
func signRequest(req *http.Request, at time.Time, region, access, secret, payloadHash string) {
	stamp := at.UTC().Format(SIGN_TIME)
	date := at.UTC().Format(SIGN_DATE)
	req.Header.Set(HEADER_DATE, stamp)
	req.Header.Set(HEADER_HASH, payloadHash)
	names := []string{"host"}
	headers := map[string]string{"host": req.Host}
	for name, values := range req.Header {
		key := strings.ToLower(name)
		// Sign every explicitly supplied header, including Range and conditional writes.
		names = append(names, key)
		headers[key] = strings.Join(strings.Fields(strings.Join(values, ",")), " ")
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, name := range names {
		fmt.Fprintf(&canonicalHeaders, "%s:%s\n", name, headers[name])
	}
	signed := strings.Join(names, ";")
	canonical := strings.Join([]string{req.Method, req.URL.EscapedPath(), req.URL.RawQuery,
		canonicalHeaders.String(), signed, payloadHash}, "\n")
	digest := sha256.Sum256([]byte(canonical))
	scope := strings.Join([]string{date, region, SIGN_SERVICE, SIGN_END}, "/")
	toSign := strings.Join([]string{SIGN_ALGORITHM, stamp, scope, hex.EncodeToString(digest[:])}, "\n")
	dateKey := hmacDigest([]byte("AWS4"+secret), date)
	regionKey := hmacDigest(dateKey, region)
	serviceKey := hmacDigest(regionKey, SIGN_SERVICE)
	signature := hmacDigest(hmacDigest(serviceKey, SIGN_END), toSign)
	req.Header.Set("Authorization", fmt.Sprintf("%s Credential=%s/%s,SignedHeaders=%s,Signature=%x",
		SIGN_ALGORITHM, access, scope, signed, signature))
}

func (store r2Store) request(method, key string, query url.Values, body io.Reader, size int64, hash string) (io.ReadCloser, error) {
	path := "/" + store.bucket
	if key != "" {
		path += "/" + key
	}
	endpoint := url.URL{Scheme: "https", Host: store.host, Path: path, RawQuery: canonicalQuery(query)}
	req, err := http.NewRequest(method, endpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create R2 %s request", method)
	}
	req.ContentLength = size
	if method == http.MethodPut {
		req.Header.Set("If-None-Match", "*")
	}
	signRequest(req, time.Now(), R2_REGION, store.access, store.secret, hash)
	response, err := store.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("R2 %s request failed", method)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		response.Body.Close()
		return nil, fmt.Errorf("R2 %s returned HTTP %d", method, response.StatusCode)
	}
	return response.Body, nil
}

func (store r2Store) list() ([]object, error) {
	query := url.Values{"list-type": {"2"}, "prefix": {OBJECT_PREFIX}}
	var objects []object
	for {
		body, err := store.request(http.MethodGet, "", query, nil, 0, EMPTY_HASH)
		if err != nil {
			return nil, err
		}
		var page struct {
			XMLName               xml.Name `xml:"ListBucketResult"`
			Contents              []object
			IsTruncated           bool
			NextContinuationToken string
		}
		err = xml.NewDecoder(body).Decode(&page)
		body.Close()
		if err != nil {
			return nil, fmt.Errorf("invalid R2 object listing")
		}
		objects = append(objects, page.Contents...)
		if !page.IsTruncated {
			return objects, nil
		}
		token := page.NextContinuationToken
		if token == "" || token == query.Get("continuation-token") {
			return nil, fmt.Errorf("R2 listing did not advance")
		}
		query.Set("continuation-token", token)
	}
}

func (store r2Store) put(key string, data io.Reader, size int64, hash string) error {
	body, err := store.request(http.MethodPut, key, nil, data, size, hash)
	if err != nil {
		return err
	}
	return body.Close()
}

func (store r2Store) get(key string) (io.ReadCloser, error) {
	return store.request(http.MethodGet, key, nil, nil, 0, EMPTY_HASH)
}

func (store r2Store) delete(key string) error {
	body, err := store.request(http.MethodDelete, key, nil, nil, 0, EMPTY_HASH)
	if err != nil {
		return err
	}
	return body.Close()
}
