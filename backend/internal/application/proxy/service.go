package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	egressapp "github.com/chenyme/grok2api/backend/internal/application/egress"
	domain "github.com/chenyme/grok2api/backend/internal/domain/proxy"
	"github.com/chenyme/grok2api/backend/internal/infra/security"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

var (
	ErrInvalidInput = errors.New("代理参数无效")
	ErrInvalidSort = errors.New("代理排序条件无效")
	ErrNotFound = errors.New("代理不存在")
	ErrInUse = errors.New("代理已被逻辑账号组使用")
)

const maxPageSize = 100

// Input 表示创建或更新通用代理时允许写入的字段。
type Input struct {
	Name     string
	Enabled  bool
	ProxyURL *string
}

// ListInput 表示管理端代理列表查询参数。
type ListInput struct {
	Page      int
	PageSize  int
	Search    string
	Enabled   *bool
	Protocol  string
	Sort      repository.SortQuery
}

// ListResult 表示代理分页列表结果。
type ListResult struct {
	Items    []domain.Endpoint
	Total    int64
	Page     int
	PageSize int
}

// ProbeResult 表示单个代理连接测试结果。
type ProbeResult struct {
	OK        bool
	LatencyMS *int64
	Error     string
	TestedAt  time.Time
}

// Prober 定义代理连接测试所需的最小基础设施能力。
type Prober interface {
	Probe(ctx context.Context, proxyURL string) (time.Duration, error)
}

// Service 负责通用代理的校验、加密、测试和绑定保护。
type Service struct {
	repository repository.ProxyRepository
	cipher     *security.Cipher
	prober     Prober
}

// NewService 创建通用代理应用服务。
// 参数 repository 为代理仓储，cipher 为敏感地址加密器，prober 为连接测试器；返回服务实例。
func NewService(repository repository.ProxyRepository, cipher *security.Cipher, prober Prober) *Service {
	return &Service{repository: repository, cipher: cipher, prober: prober}
}

// List 分页查询通用代理，响应中不暴露完整代理地址或认证信息。
// 参数 ctx 为请求上下文，input 为分页过滤条件；返回分页结果和错误。
func (s *Service) List(ctx context.Context, input ListInput) (ListResult, error) {
	if input.Page < 1 {
		input.Page = 1
	}
	if input.PageSize < 1 {
		input.PageSize = 20
	}
	if input.PageSize > maxPageSize {
		return ListResult{}, fmt.Errorf("%w: pageSize 不能超过 %d", ErrInvalidInput, maxPageSize)
	}
	if !repository.IsValidSort(input.Sort, "name", "createdAt") {
		return ListResult{}, ErrInvalidSort
	}
	protocol := strings.ToLower(strings.TrimSpace(input.Protocol))
	if protocol != "" && !isSupportedProtocol(protocol) {
		return ListResult{}, fmt.Errorf("%w: 不支持的代理协议", ErrInvalidInput)
	}
	values, total, err := s.repository.List(ctx, repository.ProxyListQuery{
		Page: repository.PageQuery{Offset: (input.Page - 1) * input.PageSize, Limit: input.PageSize, Search: input.Search, Sort: input.Sort},
		Filter: repository.ProxyListFilter{Enabled: input.Enabled, Protocol: protocol},
	})
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: values, Total: total, Page: input.Page, PageSize: input.PageSize}, nil
}

// ListEnabled 返回账号编辑器可绑定的启用代理选项。
// 参数 ctx 为请求上下文；返回不包含代理密文的启用代理列表和错误。
func (s *Service) ListEnabled(ctx context.Context) ([]domain.Endpoint, error) {
	return s.repository.ListEnabled(ctx)
}

// Create 验证并加密代理地址后创建代理资源。
// 参数 ctx 为请求上下文，input 为管理端输入；返回新代理和错误。
func (s *Service) Create(ctx context.Context, input Input) (domain.Endpoint, error) {
	value, err := s.applyInput(domain.Endpoint{}, input, true)
	if err != nil {
		return domain.Endpoint{}, err
	}
	created, err := s.repository.Create(ctx, value)
	if errors.Is(err, repository.ErrConflict) {
		return domain.Endpoint{}, fmt.Errorf("%w: 代理名称已存在", ErrInvalidInput)
	}
	return created, err
}

// Update 更新已有代理，未传 proxyURL 时保留原密文。
// 参数 ctx 为请求上下文，id 为代理标识，input 为管理端输入；返回更新结果和错误。
func (s *Service) Update(ctx context.Context, id uint64, input Input) (domain.Endpoint, error) {
	value, err := s.repository.Get(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return domain.Endpoint{}, ErrNotFound
	}
	if err != nil {
		return domain.Endpoint{}, err
	}
	value, err = s.applyInput(value, input, false)
	if err != nil {
		return domain.Endpoint{}, err
	}
	value.UpdatedAt = time.Now().UTC()
	updated, err := s.repository.Update(ctx, value)
	if errors.Is(err, repository.ErrConflict) {
		return domain.Endpoint{}, fmt.Errorf("%w: 代理名称已存在", ErrInvalidInput)
	}
	return updated, err
}

// Delete 删除未被任何逻辑账号组引用的代理。
// 参数 ctx 为请求上下文，id 为代理标识；返回不存在、占用冲突或数据库错误。
func (s *Service) Delete(ctx context.Context, id uint64) error {
	err := s.repository.Delete(ctx, id)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, repository.ErrConflict):
		return ErrInUse
	default:
		return err
	}
}

// TestConnection 通过指定代理发起一次外部请求，并持久化安全测试摘要。
// 参数 ctx 为请求上下文，id 为代理标识；返回测试结果和仅限基础设施失败的错误。
func (s *Service) TestConnection(ctx context.Context, id uint64) (ProbeResult, error) {
	value, err := s.repository.Get(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return ProbeResult{}, ErrNotFound
	}
	if err != nil {
		return ProbeResult{}, err
	}
	proxyURL, err := s.cipher.Decrypt(value.EncryptedURL)
	if err != nil {
		return ProbeResult{}, err
	}
	testedAt := time.Now().UTC()
	if s.prober == nil {
		return ProbeResult{}, errors.New("代理连接测试器未初始化")
	}
	latency, probeErr := s.prober.Probe(ctx, proxyURL)
	ok := probeErr == nil
	value.LastTestOK = &ok
	value.LastTestAt = &testedAt
	value.LastLatencyMS = nil
	value.LastTestError = ""
	if probeErr == nil {
		milliseconds := latency.Milliseconds()
		value.LastLatencyMS = &milliseconds
	} else {
		value.LastTestError = safeProbeError(probeErr)
	}
	if err := s.repository.SaveTestResult(ctx, value); err != nil {
		return ProbeResult{}, err
	}
	return ProbeResult{OK: ok, LatencyMS: value.LastLatencyMS, Error: value.LastTestError, TestedAt: testedAt}, nil
}

// applyInput 将管理端输入应用到代理领域对象，并加密敏感地址。
// 参数 value 为原代理，input 为输入，create 表示是否创建；返回规范化代理和错误。
func (s *Service) applyInput(value domain.Endpoint, input Input, create bool) (domain.Endpoint, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 160 {
		return domain.Endpoint{}, fmt.Errorf("%w: 名称必须在 1 到 160 个字符之间", ErrInvalidInput)
	}
	value.Name = name
	value.Enabled = input.Enabled
	if create && input.ProxyURL == nil {
		return domain.Endpoint{}, fmt.Errorf("%w: 代理地址不能为空", ErrInvalidInput)
	}
	if input.ProxyURL != nil {
		normalized, err := egressapp.NormalizeProxyURL(*input.ProxyURL)
		if err != nil || normalized == "" {
			return domain.Endpoint{}, fmt.Errorf("%w: 代理地址格式无效", ErrInvalidInput)
		}
		if strings.Contains(normalized, egressapp.ProxyAccountPlaceholder) {
			return domain.Endpoint{}, fmt.Errorf("%w: 逻辑账号组固定代理不能使用 {account} 占位符", ErrInvalidInput)
		}
		parsed, parseErr := url.Parse(normalized)
		if parseErr != nil || parsed.Hostname() == "" || parsed.Port() == "" {
			return domain.Endpoint{}, fmt.Errorf("%w: 代理地址必须包含主机和端口", ErrInvalidInput)
		}
		protocol := strings.ToLower(parsed.Scheme)
		if !isSupportedProtocol(protocol) {
			return domain.Endpoint{}, fmt.Errorf("%w: 仅支持 HTTP、HTTPS、SOCKS5", ErrInvalidInput)
		}
		port, parseErr := strconv.Atoi(parsed.Port())
		if parseErr != nil || port < 1 || port > 65535 || len(parsed.Hostname()) > 255 {
			return domain.Endpoint{}, fmt.Errorf("%w: 代理主机或端口无效", ErrInvalidInput)
		}
		encrypted, encryptErr := s.cipher.Encrypt(normalized)
		if encryptErr != nil {
			return domain.Endpoint{}, encryptErr
		}
		value.Protocol = protocol
		value.Host = parsed.Hostname()
		value.Port = port
		value.AuthConfigured = parsed.User != nil
		value.EncryptedURL = encrypted
	}
	return value, nil
}

// isSupportedProtocol 判断协议是否属于逻辑账号固定代理支持范围。
// 参数 value 为小写协议名；返回是否支持。
func isSupportedProtocol(value string) bool {
	return value == "http" || value == "https" || value == "socks5" || value == "socks5h"
}

// safeProbeError 将连接错误压缩为不泄露认证信息的管理端摘要。
// 参数 err 为底层连接错误；返回固定安全错误文本。
func safeProbeError(err error) string {
	if err == nil {
		return ""
	}
	return "代理连接失败"
}
