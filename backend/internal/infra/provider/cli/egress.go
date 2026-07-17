package cli

import (
	"context"
	"io"
	"net/http"

	domainegress "github.com/chenyme/grok2api/backend/internal/domain/egress"
	infraegress "github.com/chenyme/grok2api/backend/internal/infra/egress"
)

type egressTransport struct {
	manager  *infraegress.Manager
	fallback http.RoundTripper
}

func (t *egressTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if credential, exists := infraegress.CredentialFromContext(request.Context()); exists {
		lease, bound, err := t.manager.AcquireBoundCredential(request.Context(), domainegress.ScopeBuild, credential)
		if err != nil {
			return nil, err
		}
		if bound {
			return t.roundTripLease(request, lease)
		}
	}
	affinity := infraegress.AccountFromContext(request.Context())
	if affinity == "" {
		affinity = "bootstrap"
	}
	lease, configured, err := t.manager.AcquireIfConfigured(request.Context(), domainegress.ScopeBuild, affinity)
	if err != nil {
		return nil, err
	}
	if !configured {
		return t.fallback.RoundTrip(request)
	}
	return t.roundTripLease(request, lease)
}

// roundTripLease 使用已选择的出口租约执行 Build 请求并正确释放连接计数。
// 参数 request 为上游请求，lease 为出口租约；返回上游响应或传输错误。
func (t *egressTransport) roundTripLease(request *http.Request, lease *infraegress.Lease) (*http.Response, error) {
	if lease.UserAgent != "" {
		request.Header.Set("User-Agent", lease.UserAgent)
	}
	response, err := lease.Do(request)
	if err != nil {
		t.manager.FeedbackForScope(context.WithoutCancel(request.Context()), domainegress.ScopeBuild, lease.NodeID, 0, err)
		lease.Release()
		return nil, err
	}
	t.manager.FeedbackForScope(context.WithoutCancel(request.Context()), domainegress.ScopeBuild, lease.NodeID, response.StatusCode, nil)
	if response.Body == nil {
		lease.Release()
		return response, nil
	}
	response.Body = &egressResponseBody{ReadCloser: response.Body, release: lease.Release}
	return response, nil
}

type egressResponseBody struct {
	io.ReadCloser
	release func()
}

func (b *egressResponseBody) Close() error {
	err := b.ReadCloser.Close()
	if b.release != nil {
		b.release()
		b.release = nil
	}
	return err
}
