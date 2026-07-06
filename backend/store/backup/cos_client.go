package backup

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	cosUploadTimeout     = 30 * time.Minute // 1800s，支持大文件上传
	cosUploadRetryDelay  = 2 * time.Second
	cosUploadMaxAttempts = 3
	cosMultipartThreshold = 5 * 1024 * 1024 // 5MB，超过此大小使用分块上传
	cosPartSize          = 5 * 1024 * 1024  // 5MB，每个分块大小
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
		httpClient: &http.Client{Timeout: cosUploadTimeout},
		now:        time.Now,
	}
}

func (c *COSCloudStorageClient) Upload(ctx context.Context, objectKey, localPath, contentType string) error {
	// 获取文件大小，决定使用简单上传还是分块上传
	stat, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("stat local file: %w", err)
	}

	var lastErr error
	maxAttempts := cosUploadMaxAttempts

	// 根据文件大小选择上传方式
	useMultipart := stat.Size() > cosMultipartThreshold

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var err error
		if useMultipart {
				// 分块上传内部已有重试逻辑，这里只做外层重试
			err = c.uploadMultipart(ctx, objectKey, localPath, contentType)
		} else {
			err = c.uploadOnce(ctx, objectKey, localPath, contentType)
		}
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == maxAttempts || errors.Is(err, context.Canceled) || ctx.Err() != nil {
			break
		}
		log.Printf("[backup] COS upload attempt %d/%d failed for %s (multipart=%v): %v; retrying in %s",
			attempt, maxAttempts, objectKey, useMultipart, err, cosUploadRetryDelay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cosUploadRetryDelay):
		}
	}
	return lastErr
}

func (c *COSCloudStorageClient) uploadOnce(ctx context.Context, objectKey, localPath, contentType string) error {
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

// cosCompletePart 分块上传的单个分块信息
type cosCompletePart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

// 分块上传相关 XML 结构
type cosInitMultipartUploadResult struct {
	Bucket  string `xml:"Bucket"`
	Key     string `xml:"Key"`
	UploadID string `xml:"UploadId"`
	StorageClass string `xml:"StorageClass"`
}

type cosCompleteMultipartUploadResult struct {
	Location string `xml:"Location"`
	Bucket   string `xml:"Bucket"`
	Key      string `xml:"Key"`
	ETag     string `xml:"ETag"`
}

// cosCompleteMultipartUploadRequest 对应 COS CompleteMultipartUpload 请求体。
// 必须显式声明 XMLName 为 CompleteMultipartUpload，否则 encoding/xml 会用
// Go 结构体类型名作为根节点标签（如 <cosCompleteMultipartUploadRequest>），
// 不符合 COS 官方 Schema（要求根节点为 <CompleteMultipartUpload>），
// 从而导致 COS 返回 400 MalformedXML。
// 参考：https://cloud.tencent.com/document/product/436/7742
type cosCompleteMultipartUploadRequest struct {
	XMLName xml.Name          `xml:"CompleteMultipartUpload"`
	Parts   []cosCompletePart `xml:"Part"`
}

func (c *COSCloudStorageClient) resolveObjectURL(objectKey string) *url.URL {
	resolved := *c.baseURL
	resolved.Path = "/" + strings.TrimLeft(objectKey, "/")
	return &resolved
}

// initMultipartUpload 初始化分块上传
func (c *COSCloudStorageClient) initMultipartUpload(ctx context.Context, objectKey, contentType string) (string, error) {
	reqURL := c.resolveObjectURL(objectKey)
	reqURL.RawQuery = "uploads="

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build init multipart request: %w", err)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)
	c.signRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("init multipart upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return "", fmt.Errorf("init multipart upload failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result cosInitMultipartUploadResult
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode init multipart response: %w", err)
	}
	if result.UploadID == "" {
		return "", errors.New("init multipart upload returned empty upload id")
	}
	return result.UploadID, nil
}

// uploadPart 上传单个分块
func (c *COSCloudStorageClient) uploadPart(ctx context.Context, objectKey, uploadID string, partNumber int, data []byte) (string, error) {
	reqURL := c.resolveObjectURL(objectKey)
	reqURL.RawQuery = fmt.Sprintf("partNumber=%d&uploadId=%s", partNumber, url.QueryEscape(uploadID))

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL.String(), bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("build upload part request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(data))
	c.signRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload part request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return "", fmt.Errorf("upload part failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// 从响应头获取 ETag
	return resp.Header.Get("ETag"), nil
}

// completeMultipartUpload 完成分块上传
func (c *COSCloudStorageClient) completeMultipartUpload(ctx context.Context, objectKey, uploadID string, parts []cosCompletePart) error {
	reqURL := c.resolveObjectURL(objectKey)
	reqURL.RawQuery = fmt.Sprintf("uploadId=%s", url.QueryEscape(uploadID))

	body, err := xml.Marshal(cosCompleteMultipartUploadRequest{
		Parts: parts,
	})
	if err != nil {
		return fmt.Errorf("marshal complete request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build complete multipart request: %w", err)
	}
	req.Header.Set("Content-Type", "application/xml")
	req.ContentLength = int64(len(body))
	c.signRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("complete multipart upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyResp, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("complete multipart upload failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bodyResp)))
	}
	return nil
}

// abortMultipartUpload 取消分块上传
func (c *COSCloudStorageClient) abortMultipartUpload(ctx context.Context, objectKey, uploadID string) error {
	reqURL := c.resolveObjectURL(objectKey)
	reqURL.RawQuery = fmt.Sprintf("uploadId=%s", url.QueryEscape(uploadID))

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL.String(), nil)
	if err != nil {
		return fmt.Errorf("build abort multipart request: %w", err)
	}
	c.signRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("abort multipart upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("abort multipart upload failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// uploadMultipart 分块上传主方法
func (c *COSCloudStorageClient) uploadMultipart(ctx context.Context, objectKey, localPath, contentType string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat local file: %w", err)
	}

	fileSize := stat.Size()
	totalParts := int((fileSize + int64(cosPartSize) - 1) / int64(cosPartSize))

	log.Printf("[backup] COS multipart upload started: key=%s size=%d bytes parts=%d", objectKey, fileSize, totalParts)

	uploadID, err := c.initMultipartUpload(ctx, objectKey, contentType)
	if err != nil {
		return fmt.Errorf("init multipart upload: %w", err)
	}

	// 收集所有分块的 ETag
	type partResult struct {
		PartNumber int
		ETag       string
	}
	results := make([]partResult, 0, totalParts)

	buf := make([]byte, cosPartSize)
	for partNumber := 1; partNumber <= totalParts; partNumber++ {
		// 读取当前分块数据
		var partData []byte
		if partNumber < totalParts {
			// 非最后一块，读满
			n, readErr := io.ReadFull(file, buf)
			if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
				_ = c.abortMultipartUpload(ctx, objectKey, uploadID)
				return fmt.Errorf("read part %d: %w", partNumber, readErr)
			}
			partData = buf[:n]
		} else {
			// 最后一块，读取剩余所有数据
			remaining := int(fileSize) - (totalParts-1)*cosPartSize
			if remaining <= 0 {
				remaining = int(fileSize) % cosPartSize
			}
			partData = make([]byte, remaining)
			n, readErr := io.ReadFull(file, partData)
			if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
				_ = c.abortMultipartUpload(ctx, objectKey, uploadID)
				return fmt.Errorf("read last part: %w", readErr)
			}
			partData = partData[:n]
		}

		// 上传分块（带重试）
		var etag string
		var lastErr error
		for attempt := 1; attempt <= cosUploadMaxAttempts; attempt++ {
		etag, lastErr = c.uploadPart(ctx, objectKey, uploadID, partNumber, partData)
			if lastErr == nil {
				break
			}
			if attempt == cosUploadMaxAttempts {
				break
			}
			log.Printf("[backup] COS upload part %d/%d attempt %d/%d failed: %v; retrying in %s",
				partNumber, totalParts, attempt, cosUploadMaxAttempts, lastErr, cosUploadRetryDelay)
			select {
			case <-ctx.Done():
				_ = c.abortMultipartUpload(ctx, objectKey, uploadID)
				return ctx.Err()
			case <-time.After(cosUploadRetryDelay):
			}
		}
		if lastErr != nil {
			_ = c.abortMultipartUpload(ctx, objectKey, uploadID)
			return fmt.Errorf("upload part %d/%d: %w", partNumber, totalParts, lastErr)
		}

		results = append(results, partResult{PartNumber: partNumber, ETag: etag})
		log.Printf("[backup] COS uploaded part %d/%d", partNumber, totalParts)
	}

	// 完成分块上传
	completeParts := make([]cosCompletePart, len(results))
	for i, r := range results {
		completeParts[i] = cosCompletePart{PartNumber: r.PartNumber, ETag: r.ETag}
	}

	if err := c.completeMultipartUpload(ctx, objectKey, uploadID, completeParts); err != nil {
		_ = c.abortMultipartUpload(ctx, objectKey, uploadID)
		return fmt.Errorf("complete multipart upload: %w", err)
	}

	log.Printf("[backup] COS multipart upload completed: key=%s", objectKey)
	return nil
}

// PresignGetURL 为指定对象生成带签名的临时 GET 访问 URL。
// 签名参数附加在 query 上，调用方可直接把返回值作为 <img src> 使用。
// expire 为有效期，<=0 时使用默认 15 分钟。
func (c *COSCloudStorageClient) PresignGetURL(objectKey string, expire time.Duration) (string, error) {
	return c.PresignGetURLWithProcess(objectKey, expire, "")
}

// PresignGetURLWithProcess 在 PresignGetURL 基础上支持附加数据万象图片处理参数
// （如 imageMogr2/format/webp、imageMogr2/thumbnail/!30p）。
//
// 重要：imageMogr2 是一个"路径式无值参数"，在 URL 中必须保持原样（斜杠/叹号不转义），
// 且 COS 下载时处理 **不要求** 该参数参与签名 —— 因此签名仍只覆盖 host，
// q-url-param-list 为空，处理参数以原始字符串直接拼到最终 query 末尾。
// 这样可避免 url.Values.Encode() 把 "imageMogr2/.../!30p" 转义成 "imageMogr2%2F...%2130p"
// 而导致 URL 与签名串编码不一致引发的 SignatureDoesNotMatch。
//
// process 形如 "imageMogr2/format/webp"，为空则等价于普通预签名 URL。
func (c *COSCloudStorageClient) PresignGetURLWithProcess(objectKey string, expire time.Duration, process string) (string, error) {
	if c.baseURL == nil {
		return "", errors.New("cos client base url is not configured")
	}
	if strings.TrimSpace(c.secretID) == "" || strings.TrimSpace(c.secretKey) == "" {
		return "", errors.New("cos client credentials are not configured")
	}
	if expire <= 0 {
		expire = 15 * time.Minute
	}

	target := c.resolveObjectURL(objectKey)

	now := c.now()
	start := now.Add(-time.Minute).Unix()
	end := now.Add(expire).Unix()
	keyTime := fmt.Sprintf("%d;%d", start, end)

	// 预签名 GET：header-list 只签 host，query-param-list 为空（处理参数不参与签名）。
	headerPairs := "host=" + cosEscape(target.Host)
	headerNames := []string{"host"}

	httpString := "get\n" + canonicalPath(target) + "\n\n" + headerPairs + "\n"
	stringToSign := "sha1\n" + keyTime + "\n" + sha1Hex(httpString) + "\n"
	signKey := hmacSHA1Hex(c.secretKey, keyTime)
	signature := hmacSHA1Hex(signKey, stringToSign)

	// 用 url.Values 仅承载签名参数，保证它们被正确编码。
	signQuery := url.Values{}
	signQuery.Set("q-sign-algorithm", "sha1")
	signQuery.Set("q-ak", c.secretID)
	signQuery.Set("q-sign-time", keyTime)
	signQuery.Set("q-key-time", keyTime)
	signQuery.Set("q-header-list", strings.Join(headerNames, ";"))
	signQuery.Set("q-url-param-list", "")
	signQuery.Set("q-signature", signature)

	rawQuery := signQuery.Encode()
	// 图片处理参数作为原始无值参数前置拼接，保持斜杠/叹号不被转义。
	if p := strings.TrimSpace(process); p != "" {
		rawQuery = strings.TrimLeft(p, "?") + "&" + rawQuery
	}
	target.RawQuery = rawQuery

	return target.String(), nil
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

	// 按 COS 规范：参数名转小写后排序，签名值用 lowerName=escape(value)。
	// 用 lowerName -> originalName 映射回原始 key 以正确取值（如 imageMogr2 含大写）。
	lowerToOriginal := make(map[string]string, len(values))
	names := make([]string, 0, len(values))
	for name := range values {
		lower := strings.ToLower(name)
		lowerToOriginal[lower] = name
		names = append(names, lower)
	}
	sort.Strings(names)

	parts := make([]string, 0)
	for _, name := range names {
		originalValues := values[lowerToOriginal[name]]
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
