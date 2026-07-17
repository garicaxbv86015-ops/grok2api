package proxy

import (
	"errors"
	"testing"

	proxydomain "github.com/chenyme/grok2api/backend/internal/domain/proxy"
	"github.com/chenyme/grok2api/backend/internal/infra/security"
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
