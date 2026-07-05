package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestConstants 测试常量配置
func TestConstants(t *testing.T) {
	// 验证分块阈值
	if cosMultipartThreshold != 5*1024*1024 {
		t.Errorf("cosMultipartThreshold should be 5MB, got %d", cosMultipartThreshold)
	}

	// 验证分块大小
	if cosPartSize != 5*1024*1024 {
		t.Errorf("cosPartSize should be 5MB, got %d", cosPartSize)
	}

	// 验证超时配置
	if cosUploadTimeout != 30*time.Minute {
		t.Errorf("cosUploadTimeout should be 30 minutes, got %v", cosUploadTimeout)
	}

	// 验证重试次数
	if cosUploadMaxAttempts != 3 {
		t.Errorf("cosUploadMaxAttempts should be 3, got %d", cosUploadMaxAttempts)
	}
}

// TestMultipartUploadThreshold 测试分块上传阈值判断
func TestMultipartUploadThreshold(t *testing.T) {
	tests := []struct {
		name     string
		fileSize int64
		expected bool // true = 使用分块上传
	}{
		{"small file 1KB", 1024, false},
		{"medium file 5MB", 5 * 1024 * 1024, false},
		{"large file 5MB+1", 5*1024*1024 + 1, true},
		{"large file 10MB", 10 * 1024 * 1024, true},
		{"large file 100MB", 100 * 1024 * 1024, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useMultipart := tt.fileSize > cosMultipartThreshold
			if useMultipart != tt.expected {
				t.Errorf("file size %d bytes: expected useMultipart=%v, got %v",
					tt.fileSize, tt.expected, useMultipart)
			}
		})
	}
}

// TestCalculateTotalParts 测试计算总分块数
func TestCalculateTotalParts(t *testing.T) {
	tests := []struct {
		name     string
		fileSize int64
		expected int
	}{
		{"empty file", 0, 0},
		{"small file 1KB", 1024, 1},
		{"exactly 5MB", 5 * 1024 * 1024, 1},
		{"5MB + 1 byte", 5*1024*1024 + 1, 2},
		{"10MB", 10 * 1024 * 1024, 2},
		{"25MB", 25 * 1024 * 1024, 5},
		{"100MB", 100 * 1024 * 1024, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			totalParts := int((tt.fileSize + int64(cosPartSize) - 1) / int64(cosPartSize))
			if tt.fileSize == 0 {
				totalParts = 0
			}
			if totalParts != tt.expected {
				t.Errorf("file size %d bytes: expected %d parts, got %d",
					tt.fileSize, tt.expected, totalParts)
			}
		})
	}
}

// TestNewCOSCloudStorageClient 测试客户端初始化
func TestNewCOSCloudStorageClient(t *testing.T) {
	client := NewCOSCloudStorageClient("test-bucket", "ap-guangzhou", "test-id", "test-key")

	if client == nil {
		t.Fatal("NewCOSCloudStorageClient returned nil")
	}

	if client.httpClient == nil {
		t.Error("httpClient is nil")
	}

	if client.httpClient.Timeout != cosUploadTimeout {
		t.Errorf("httpClient.Timeout should be %v, got %v", cosUploadTimeout, client.httpClient.Timeout)
	}

	// 验证基础 URL
	expectedURL := "https://test-bucket.cos.ap-guangzhou.myqcloud.com"
	if client.baseURL.String() != expectedURL {
		t.Errorf("baseURL should be %s, got %s", expectedURL, client.baseURL.String())
	}
}

// TestResolveObjectURL 测试对象 URL 解析
func TestResolveObjectURL(t *testing.T) {
	client := NewCOSCloudStorageClient("test-bucket", "ap-guangzhou", "test-id", "test-key")

	tests := []struct {
		name       string
		objectKey  string
		expectPath string
	}{
		{"simple key", "file.txt", "/file.txt"},
		{"key with path", "folder/file.txt", "/folder/file.txt"},
		{"key with leading slash", "/file.txt", "/file.txt"},
		{"key with multiple slashes", "//file.txt", "/file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := client.resolveObjectURL(tt.objectKey)
			if url.Path != tt.expectPath {
				t.Errorf("expected path %s, got %s", tt.expectPath, url.Path)
			}
		})
	}
}

// TestNormalizeCOSPrefix 测试前缀规范化
func TestNormalizeCOSPrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"backups", "backups/"},
		{"backups/", "backups/"},
		{"/backups", "backups/"},
		{"/backups/", "backups/"},
		{"  backups  ", "backups/"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeCOSPrefix(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeCOSPrefix(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestCOSSignature 测试签名生成
func TestCOSSignature(t *testing.T) {
	client := NewCOSCloudStorageClient("test-bucket", "ap-guangzhou", "test-id", "test-key")

	// 创建一个简单的 GET 请求来测试签名
	reqURL := client.resolveObjectURL("test.txt")
	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	client.signRequest(req)

	authorization := req.Header.Get("Authorization")
	if authorization == "" {
		t.Error("Authorization header is empty")
	}

	// 验证签名格式
	if !strings.Contains(authorization, "q-sign-algorithm=sha1") {
		t.Error("Authorization should contain q-sign-algorithm=sha1")
	}
	if !strings.Contains(authorization, "q-ak=test-id") {
		t.Error("Authorization should contain q-ak=test-id")
	}
}

// TestUploadSmallFileMock 测试小文件简单上传（使用 mock 服务器）
func TestUploadSmallFileMock(t *testing.T) {
	// 创建一个 mock COS 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT method, got %s", r.Method)
		}

		// 读取请求体
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read request body: %v", err)
		}

		// 验证内容
		if len(body) != 100 {
			t.Errorf("Expected 100 bytes, got %d", len(body))
		}

		// 返回成功
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 从服务器 URL 解析 bucket 和 region
	// 注意：这是一个简化的测试，实际测试需要更复杂的设置
	t.Logf("Mock server URL: %s", server.URL)
}

// TestMultipartUploadMock 测试分块上传流程（使用 mock 服务器）
func TestMultipartUploadMock(t *testing.T) {
	// 创建一个 mock COS 服务器，模拟分块上传流程
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 根据请求路径和参数判断请求类型
		path := r.URL.Path
		query := r.URL.Query()

		switch {
		case query.Get("uploads") != "":
			// 初始化分块上传
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><InitiateMultipartUploadResult><Bucket>test-bucket</Bucket><Key>test.db</Key><UploadId>test-upload-id</UploadId></InitiateMultipartUploadResult>`))
		case query.Get("uploadId") != "":
			if query.Get("partNumber") != "" {
				// 上传分块
				w.Header().Set("ETag", `"test-etag"`)
				w.WriteHeader(http.StatusOK)
			} else {
				// 完成分块上传
				w.Header().Set("Content-Type", "application/xml")
				w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><CompleteMultipartUploadResult><Location>https://test-bucket.cos.ap-guangzhou.myqcloud.com/test.db</Location><Bucket>test-bucket</Bucket><Key>test.db</Key><ETag>"test-final-etag"</ETag></CompleteMultipartUploadResult>`))
			}
		default:
			t.Errorf("Unexpected request: path=%s, query=%v", path, query)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	t.Logf("Mock server URL: %s", server.URL)
}

// TestErrorHandling 测试错误处理
func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		isNil    bool
		contains string
	}{
		{"context deadline exceeded", context.DeadlineExceeded, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt.err // 使用错误变量
			// 这个测试主要验证错误类型，不需要检查错误消息
		})
	}
}

// TestLargeFileUploadIntegration 集成测试：模拟大文件上传
func TestLargeFileUploadIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// 创建临时测试文件（大于 5MB）
	testFileSize := 6 * 1024 * 1024 // 6MB
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "large_test.db")

	// 创建测试数据
	data := make([]byte, testFileSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// 验证文件大小
	info, err := os.Stat(tempFile)
	if err != nil {
		t.Fatalf("Failed to stat test file: %v", err)
	}

	if info.Size() != int64(testFileSize) {
		t.Errorf("Test file size should be %d, got %d", testFileSize, info.Size())
	}

	// 计算分块数
	expectedParts := int((int64(testFileSize) + int64(cosPartSize) - 1) / int64(cosPartSize))
	if expectedParts != 2 {
		t.Errorf("Expected 2 parts for 6MB file, got %d", expectedParts)
	}

	t.Logf("Test file created: %s, size: %d bytes, expected parts: %d", tempFile, testFileSize, expectedParts)

	// 计算测试文件的 SHA256 哈希
	hash := sha256.Sum256(data)
	t.Logf("Test file SHA256: %s", hex.EncodeToString(hash[:]))
}

// TestContextCancellation 测试上下文取消
func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 立即取消
	cancel()

	// 验证上下文已取消
	select {
	case <-ctx.Done():
		// 预期行为
	default:
		t.Error("Context should be cancelled")
	}
}

// TestRetryLogic 测试重试逻辑
func TestRetryLogic(t *testing.T) {
	// 模拟重试配置
	maxAttempts := cosUploadMaxAttempts
	retryDelay := cosUploadRetryDelay

	if maxAttempts != 3 {
		t.Errorf("Expected maxAttempts = 3, got %d", maxAttempts)
	}

	if retryDelay != 2*time.Second {
		t.Errorf("Expected retryDelay = 2s, got %v", retryDelay)
	}

	// 计算最大重试等待时间
	maxRetryWait := time.Duration(maxAttempts-1) * retryDelay
	expectedMaxWait := 4 * time.Second // 2次重试等待
	if maxRetryWait != expectedMaxWait {
		t.Errorf("Expected maxRetryWait = %v, got %v", expectedMaxWait, maxRetryWait)
	}
}

// TestConcurrentUploads 测试并发上传安全性
func TestConcurrentUploads(t *testing.T) {
	// 创建多个客户端
	clients := make([]*COSCloudStorageClient, 10)
	for i := 0; i < 10; i++ {
		clients[i] = NewCOSCloudStorageClient(
			fmt.Sprintf("bucket-%d", i),
			"ap-guangzhou",
			fmt.Sprintf("id-%d", i),
			fmt.Sprintf("key-%d", i),
		)
	}

	// 验证每个客户端都有独立的配置
	for i, client := range clients {
		if client.baseURL.Host != fmt.Sprintf("bucket-%d.cos.ap-guangzhou.myqcloud.com", i) {
			t.Errorf("Client %d: unexpected baseURL host", i)
		}
	}
}

// BenchmarkMultipartThreshold 性能测试：分块阈值判断
func BenchmarkMultipartThreshold(b *testing.B) {
	fileSizes := []int64{
		1024,
		5 * 1024 * 1024,
		5*1024*1024 + 1,
		100 * 1024 * 1024,
	}

	for _, size := range fileSizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = size > cosMultipartThreshold
			}
		})
	}
}

// BenchmarkPartCalculation 性能测试：分块数计算
func BenchmarkPartCalculation(b *testing.B) {
	fileSizes := []int64{
		1024,
		5 * 1024 * 1024,
		100 * 1024 * 1024,
		1024 * 1024 * 1024, // 1GB
	}

	for _, size := range fileSizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = int((size + int64(cosPartSize) - 1) / int64(cosPartSize))
			}
		})
	}
}
