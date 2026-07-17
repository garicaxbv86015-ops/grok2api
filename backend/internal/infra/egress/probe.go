package egress

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultProxyProbeURL = "https://www.cloudflare.com/cdn-cgi/trace"

// ProxyProber 通过真实 HTTP 请求检查通用代理能否建立外部连接。
type ProxyProber struct {
	targetURL string
	timeout   time.Duration
}

// NewProxyProber 创建使用固定安全探测地址的代理测试器。
// 参数 timeout 为单次测试超时；返回代理测试器。
func NewProxyProber(timeout time.Duration) *ProxyProber {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ProxyProber{targetURL: defaultProxyProbeURL, timeout: timeout}
}

// Probe 通过代理访问探测地址并返回端到端耗时。
// 参数 ctx 为请求上下文，proxyURL 为已解密代理地址；返回耗时和连接错误。
func (p *ProxyProber) Probe(ctx context.Context, proxyURL string) (time.Duration, error) {
	client, err := newBuildClient(proxyURL)
	if err != nil {
		return 0, err
	}
	client.Timeout = p.timeout
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.targetURL, nil)
	if err != nil {
		return 0, err
	}
	startedAt := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 500 {
		return 0, fmt.Errorf("探测地址返回状态码 %d", response.StatusCode)
	}
	return time.Since(startedAt), nil
}
