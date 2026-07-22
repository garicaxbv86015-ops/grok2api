package proxy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	proxydomain "github.com/chenyme/grok2api/backend/internal/domain/proxy"
	"github.com/chenyme/grok2api/backend/internal/infra/security"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

// TestApplyInputEncryptsWriteOnlyProxyURL 验证完整代理地址加密保存且只提取安全展示字段。
func TestApplyInputEncryptsWriteOnlyProxyURL(t *testing.T) {
	cipher, err := security.NewCipher("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(nil, cipher, nil)
	proxyURL := "socks5://secret:password@127.0.0.1:1080"
	value, err := service.applyInput(proxydomain.Endpoint{}, Input{Name: "本地代理", Enabled: true, ProxyURL: &proxyURL}, true)
	if err != nil {
		t.Fatal(err)
	}
	if value.Protocol != "socks5" || value.Host != "127.0.0.1" || value.Port != 1080 || !value.AuthConfigured || value.EncryptedURL == "" || value.EncryptedURL == proxyURL {
		t.Fatalf("proxy = %#v", value)
	}
	decrypted, err := cipher.Decrypt(value.EncryptedURL)
	if err != nil || decrypted != proxyURL {
		t.Fatalf("decrypted = %q, err = %v", decrypted, err)
	}
}

// TestApplyInputRejectsAccountTemplate 验证逻辑账号组固定代理不会因 Provider 身份模板产生不同出口。
func TestApplyInputRejectsAccountTemplate(t *testing.T) {
	cipher, err := security.NewCipher("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(nil, cipher, nil)
	proxyURL := "socks5h://Default.{account}:password@127.0.0.1:1080"
	if _, err := service.applyInput(proxydomain.Endpoint{}, Input{Name: "模板代理", Enabled: true, ProxyURL: &proxyURL}, true); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v", err)
	}
}

// TestTestAllConnectionsAggregatesSuccessAndFailure 验证批量测试会并发探测并汇总成功失败数。
func TestTestAllConnectionsAggregatesSuccessAndFailure(t *testing.T) {
	cipher, err := security.NewCipher("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if err != nil {
		t.Fatal(err)
	}
	okURL := "socks5://ok@127.0.0.1:1080"
	failURL := "socks5://fail@127.0.0.1:1081"
	okEncrypted, err := cipher.Encrypt(okURL)
	if err != nil {
		t.Fatal(err)
	}
	failEncrypted, err := cipher.Encrypt(failURL)
	if err != nil {
		t.Fatal(err)
	}
	repo := &memoryProxyRepository{items: []proxydomain.Endpoint{
		{ID: 1, Name: "可用", Protocol: "socks5", Host: "127.0.0.1", Port: 1080, EncryptedURL: okEncrypted, Enabled: true},
		{ID: 2, Name: "失败", Protocol: "socks5", Host: "127.0.0.1", Port: 1081, EncryptedURL: failEncrypted, Enabled: true},
	}}
	prober := &scriptedProber{results: map[string]error{
		okURL:   nil,
		failURL: errors.New("dial timeout"),
	}}
	service := NewService(repo, cipher, prober)
	result, err := service.TestAllConnections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.Succeeded != 1 || result.Failed != 1 || len(result.Items) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if repo.saved != 2 {
		t.Fatalf("saved test results = %d", repo.saved)
	}
}

type scriptedProber struct {
	results map[string]error
}

func (p *scriptedProber) Probe(_ context.Context, proxyURL string) (time.Duration, error) {
	if err, exists := p.results[proxyURL]; exists {
		if err != nil {
			return 0, err
		}
		return 42 * time.Millisecond, nil
	}
	return 0, errors.New("unexpected proxy")
}

type memoryProxyRepository struct {
	mu    sync.Mutex
	items []proxydomain.Endpoint
	saved int
}

func (r *memoryProxyRepository) List(_ context.Context, query repository.ProxyListQuery) ([]proxydomain.Endpoint, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	start := query.Page.Offset
	if start > len(r.items) {
		return nil, int64(len(r.items)), nil
	}
	end := start + query.Page.Limit
	if end > len(r.items) || query.Page.Limit <= 0 {
		end = len(r.items)
	}
	return append([]proxydomain.Endpoint(nil), r.items[start:end]...), int64(len(r.items)), nil
}

func (r *memoryProxyRepository) ListEnabled(context.Context) ([]proxydomain.Endpoint, error) {
	return nil, errors.New("not implemented")
}

func (r *memoryProxyRepository) Get(_ context.Context, id uint64) (proxydomain.Endpoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.items {
		if item.ID == id {
			return item, nil
		}
	}
	return proxydomain.Endpoint{}, repository.ErrNotFound
}

func (r *memoryProxyRepository) Create(context.Context, proxydomain.Endpoint) (proxydomain.Endpoint, error) {
	return proxydomain.Endpoint{}, errors.New("not implemented")
}

func (r *memoryProxyRepository) Update(context.Context, proxydomain.Endpoint) (proxydomain.Endpoint, error) {
	return proxydomain.Endpoint{}, errors.New("not implemented")
}

func (r *memoryProxyRepository) Delete(context.Context, uint64) error {
	return errors.New("not implemented")
}

func (r *memoryProxyRepository) SaveTestResult(_ context.Context, value proxydomain.Endpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, item := range r.items {
		if item.ID == value.ID {
			r.items[index].LastTestOK = value.LastTestOK
			r.items[index].LastLatencyMS = value.LastLatencyMS
			r.items[index].LastTestError = value.LastTestError
			r.items[index].LastTestAt = value.LastTestAt
			r.saved++
			return nil
		}
	}
	return repository.ErrNotFound
}

func (r *memoryProxyRepository) CountFamilies(context.Context, uint64) (int64, error) {
	return 0, nil
}
