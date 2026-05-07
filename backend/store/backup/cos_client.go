package backup

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

type COSCloudStorageClient struct {
	baseURL    *url.URL
	secretID   string
	secretKey  string
	httpClient *http.Client
	now        func() time.Time
}

func NewCOSCloudStorageClient(bucket, region, secretID, secretKey string) *COSCloudStorageClient {
	endpoint := fmt.Sprintf("https://%s.cos.%s.myqcloud.com", strings.TrimSpace(bucket), strings.TrimSpace(region))
	baseURL, _ := url.Parse(endpoint)
	return &COSCloudStorageClient{
		baseURL:    baseURL,
		secretID:   strings.TrimSpace(secretID),
		secretKey:  strings.TrimSpace(secretKey),
		httpClient: &http.Client{Timeout: 60 * time.Second},
		now:        time.Now,
	}
}

func (c *COSCloudStorageClient) Upload(ctx context.Context, objectKey, localPath, contentType string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat local file: %w", err)
	}

	reqURL := c.resolveObjectURL(objectKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL.String(), file)
	if err != nil {
		return fmt.Errorf("build put request: %w", err)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = stat.Size()
	c.signRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("put object request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("put object failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *COSCloudStorageClient) List(ctx context.Context, prefix string) ([]CloudObjectInfo, error) {
	var (
		marker  string
		objects []CloudObjectInfo
	)

	for {
		query := url.Values{}
		query.Set("prefix", prefix)
		query.Set("max-keys", "1000")
		if marker != "" {
			query.Set("marker", marker)
		}

		reqURL := *c.baseURL
		reqURL.Path = "/"
		reqURL.RawQuery = query.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("build list request: %w", err)
		}
		c.signRequest(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list objects request: %w", err)
		}

		var payload cosListBucketResult
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
			resp.Body.Close()
			return nil, fmt.Errorf("list objects failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		if err := xml.NewDecoder(resp.Body).Decode(&payload); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode list response: %w", err)
		}
		resp.Body.Close()

		for _, item := range payload.Contents {
			objects = append(objects, CloudObjectInfo{Key: item.Key, Size: item.Size})
		}
		if !payload.IsTruncated {
			break
		}
		marker = payload.NextMarker
		if marker == "" && len(payload.Contents) > 0 {
			marker = payload.Contents[len(payload.Contents)-1].Key
		}
		if marker == "" {
			break
		}
	}

	return objects, nil
}

type cosListBucketResult struct {
	Contents []struct {
		Key  string `xml:"Key"`
		Size int64  `xml:"Size"`
	} `xml:"Contents"`
	IsTruncated bool   `xml:"IsTruncated"`
	NextMarker  string `xml:"NextMarker"`
}

func (c *COSCloudStorageClient) resolveObjectURL(objectKey string) *url.URL {
	resolved := *c.baseURL
	resolved.Path = "/" + strings.TrimLeft(objectKey, "/")
	return &resolved
}

func (c *COSCloudStorageClient) signRequest(req *http.Request) {
	keyTime := c.signatureTimeWindow()
	headerPairs, headerNames := canonicalHeaderPairs(req)
	queryPairs, queryNames := canonicalQueryPairs(req.URL.Query())

	httpString := strings.ToLower(req.Method) + "\n" + canonicalPath(req.URL) + "\n" + queryPairs + "\n" + headerPairs + "\n"
	stringToSign := "sha1\n" + keyTime + "\n" + sha1Hex(httpString) + "\n"
	signKey := hmacSHA1Hex(c.secretKey, keyTime)
	signature := hmacSHA1Hex(signKey, stringToSign)

	req.Header.Set("Authorization", fmt.Sprintf(
		"q-sign-algorithm=sha1&q-ak=%s&q-sign-time=%s&q-key-time=%s&q-header-list=%s&q-url-param-list=%s&q-signature=%s",
		c.secretID,
		keyTime,
		keyTime,
		strings.Join(headerNames, ";"),
		strings.Join(queryNames, ";"),
		signature,
	))
	if req.Header.Get("Host") == "" {
		req.Header.Set("Host", req.URL.Host)
	}
}

func (c *COSCloudStorageClient) signatureTimeWindow() string {
	now := c.now()
	start := now.Add(-time.Minute).Unix()
	end := now.Add(15 * time.Minute).Unix()
	return fmt.Sprintf("%d;%d", start, end)
}

func canonicalHeaderPairs(req *http.Request) (string, []string) {
	pairs := map[string]string{
		"host": req.URL.Host,
	}
	if contentType := strings.TrimSpace(req.Header.Get("Content-Type")); contentType != "" {
		pairs["content-type"] = contentType
	}
	names := make([]string, 0, len(pairs))
	for name := range pairs {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+cosEscape(pairs[name]))
	}
	return strings.Join(parts, "&"), names
}

func canonicalQueryPairs(values url.Values) (string, []string) {
	if len(values) == 0 {
		return "", nil
	}

	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, strings.ToLower(name))
	}
	sort.Strings(names)

	parts := make([]string, 0)
	for _, name := range names {
		originalValues := values[name]
		if len(originalValues) == 0 {
			parts = append(parts, name+"=")
			continue
		}
		sortedValues := append([]string(nil), originalValues...)
		sort.Strings(sortedValues)
		for _, value := range sortedValues {
			parts = append(parts, name+"="+cosEscape(value))
		}
	}
	return strings.Join(parts, "&"), names
}

func canonicalPath(u *url.URL) string {
	path := u.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func cosEscape(raw string) string {
	escaped := url.QueryEscape(strings.TrimSpace(raw))
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}

func sha1Hex(raw string) string {
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func hmacSHA1Hex(key, raw string) string {
	h := hmac.New(sha1.New, []byte(key))
	_, _ = h.Write([]byte(raw))
	return hex.EncodeToString(h.Sum(nil))
}
