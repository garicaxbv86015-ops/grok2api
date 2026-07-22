package proxy

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	proxyapp "github.com/chenyme/grok2api/backend/internal/application/proxy"
	domain "github.com/chenyme/grok2api/backend/internal/domain/proxy"
	"github.com/chenyme/grok2api/backend/internal/repository"
	"github.com/chenyme/grok2api/backend/internal/shared/response"
	"github.com/gin-gonic/gin"
)

// Handler 提供通用代理管理端 HTTP 接口。
type Handler struct{ service *proxyapp.Service }

// NewHandler 创建通用代理 HTTP 处理器。
// 参数 service 为代理应用服务；返回处理器。
func NewHandler(service *proxyapp.Service) *Handler { return &Handler{service: service} }

// Register 注册通用代理管理端路由。
// 参数 router 为管理员鉴权后的路由组；无返回值。
func (h *Handler) Register(router *gin.RouterGroup) {
	router.GET("/proxies", h.list)
	router.GET("/proxies/options", h.options)
	router.POST("/proxies", h.create)
	router.POST("/proxies/test-all", h.testAllConnections)
	router.PUT("/proxies/:id", h.update)
	router.DELETE("/proxies/:id", h.delete)
	router.POST("/proxies/:id/test", h.testConnection)
}

type proxyRequest struct {
	Name     string  `json:"name"`
	Enabled  bool    `json:"enabled"`
	ProxyURL *string `json:"proxyURL"`
}

type proxyResponse struct {
	ID               uint64     `json:"id,string"`
	Name             string     `json:"name"`
	Protocol         string     `json:"protocol"`
	Address          string     `json:"address"`
	AuthConfigured   bool       `json:"authConfigured"`
	Enabled          bool       `json:"enabled"`
	LastTestOK       *bool      `json:"lastTestOK,omitempty"`
	LastLatencyMS    *int64     `json:"lastLatencyMS,omitempty"`
	LastTestError    string     `json:"lastTestError,omitempty"`
	LastTestAt       *time.Time `json:"lastTestAt,omitempty"`
	BoundFamilyCount int64      `json:"boundFamilyCount"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type batchProbeItemResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OK        bool      `json:"ok"`
	LatencyMS *int64    `json:"latencyMS,omitempty"`
	Error     string    `json:"error"`
	TestedAt  time.Time `json:"testedAt"`
}

type batchProbeResponse struct {
	Total     int                      `json:"total"`
	Succeeded int                      `json:"succeeded"`
	Failed    int                      `json:"failed"`
	Items     []batchProbeItemResponse `json:"items"`
}

// list 返回通用代理分页列表。
// 参数 c 为 Gin 请求上下文；响应直接写入上下文，无返回值。
func (h *Handler) list(c *gin.Context) {
	page, pageSize, ok := parsePagination(c)
	if !ok {
		return
	}
	var enabled *bool
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "invalidEnabled", "enabled 必须是 true 或 false")
			return
		}
		enabled = &value
	}
	result, err := h.service.List(c.Request.Context(), proxyapp.ListInput{
		Page: page, PageSize: pageSize, Search: c.Query("search"), Enabled: enabled, Protocol: c.Query("protocol"),
		Sort: repository.SortQuery{Field: c.Query("sortBy"), Direction: repository.SortDirection(c.Query("sortOrder"))},
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	items := make([]proxyResponse, 0, len(result.Items))
	for _, value := range result.Items {
		items = append(items, newProxyResponse(value))
	}
	response.Success(c, http.StatusOK, gin.H{"items": items, "total": result.Total, "page": result.Page, "pageSize": result.PageSize})
}

// options 返回账号编辑器可绑定的启用代理选项。
// 参数 c 为 Gin 请求上下文；响应直接写入上下文，无返回值。
func (h *Handler) options(c *gin.Context) {
	values, err := h.service.ListEnabled(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "proxyOptionsFailed", "读取代理选项失败")
		return
	}
	items := make([]proxyResponse, 0, len(values))
	for _, value := range values {
		items = append(items, newProxyResponse(value))
	}
	response.Success(c, http.StatusOK, gin.H{"items": items})
}

// create 创建通用代理。
// 参数 c 为 Gin 请求上下文；响应直接写入上下文，无返回值。
func (h *Handler) create(c *gin.Context) {
	var request proxyRequest
	if c.ShouldBindJSON(&request) != nil {
		response.Error(c, http.StatusBadRequest, "invalidRequest", "请求参数无效")
		return
	}
	value, err := h.service.Create(c.Request.Context(), proxyapp.Input{Name: request.Name, Enabled: request.Enabled, ProxyURL: request.ProxyURL})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, newProxyResponse(value))
}

// update 更新通用代理，proxyURL 缺省时保留原认证信息。
// 参数 c 为 Gin 请求上下文；响应直接写入上下文，无返回值。
func (h *Handler) update(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	var request proxyRequest
	if c.ShouldBindJSON(&request) != nil {
		response.Error(c, http.StatusBadRequest, "invalidRequest", "请求参数无效")
		return
	}
	value, err := h.service.Update(c.Request.Context(), id, proxyapp.Input{Name: request.Name, Enabled: request.Enabled, ProxyURL: request.ProxyURL})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, newProxyResponse(value))
}

// delete 删除未被逻辑账号组引用的通用代理。
// 参数 c 为 Gin 请求上下文；响应直接写入上下文，无返回值。
func (h *Handler) delete(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, gin.H{"deleted": true})
}

// testConnection 测试单个通用代理的外部连接能力。
// 参数 c 为 Gin 请求上下文；响应直接写入上下文，无返回值。
func (h *Handler) testConnection(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	result, err := h.service.TestConnection(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, gin.H{"ok": result.OK, "latencyMS": result.LatencyMS, "error": result.Error, "testedAt": result.TestedAt})
}

// testAllConnections 并发测试全部通用代理的外部连接能力。
// 参数 c 为 Gin 请求上下文；响应直接写入上下文，无返回值。
func (h *Handler) testAllConnections(c *gin.Context) {
	result, err := h.service.TestAllConnections(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	items := make([]batchProbeItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, batchProbeItemResponse{
			ID: strconv.FormatUint(item.ID, 10), Name: item.Name, OK: item.OK,
			LatencyMS: item.LatencyMS, Error: item.Error, TestedAt: item.TestedAt,
		})
	}
	response.Success(c, http.StatusOK, batchProbeResponse{
		Total: result.Total, Succeeded: result.Succeeded, Failed: result.Failed, Items: items,
	})
}

// newProxyResponse 将代理领域对象转换为不含敏感地址的响应。
// 参数 value 为代理领域对象；返回管理端响应对象。
func newProxyResponse(value domain.Endpoint) proxyResponse {
	return proxyResponse{
		ID: value.ID, Name: value.Name, Protocol: value.Protocol, Address: value.Host + ":" + strconv.Itoa(value.Port),
		AuthConfigured: value.AuthConfigured, Enabled: value.Enabled, LastTestOK: value.LastTestOK,
		LastLatencyMS: value.LastLatencyMS, LastTestError: value.LastTestError, LastTestAt: value.LastTestAt,
		BoundFamilyCount: value.BoundFamilyCount, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

// parsePagination 解析并限制管理端分页参数。
// 参数 c 为 Gin 请求上下文；返回页码、每页数量和是否有效。
func parsePagination(c *gin.Context) (int, int, bool) {
	page, err := strconv.Atoi(defaultString(c.Query("page"), "1"))
	if err != nil || page < 1 {
		response.Error(c, http.StatusBadRequest, "invalidPage", "page 必须是正整数")
		return 0, 0, false
	}
	pageSize, err := strconv.Atoi(defaultString(c.Query("pageSize"), "20"))
	if err != nil || pageSize < 1 {
		response.Error(c, http.StatusBadRequest, "invalidPageSize", "pageSize 必须是正整数")
		return 0, 0, false
	}
	return page, pageSize, true
}

// parsePathID 解析代理路径标识。
// 参数 c 为 Gin 请求上下文；返回代理标识和是否有效。
func parsePathID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, "invalidId", "ID 无效")
		return 0, false
	}
	return id, true
}

// defaultString 在字符串为空时返回默认值。
// 参数 value 为原始值，fallback 为默认值；返回最终字符串。
func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// writeError 将代理应用错误映射为稳定 HTTP 错误响应。
// 参数 c 为 Gin 请求上下文，err 为应用错误；响应直接写入上下文，无返回值。
func (h *Handler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, proxyapp.ErrInvalidInput), errors.Is(err, proxyapp.ErrInvalidSort):
		response.Error(c, http.StatusBadRequest, "invalidProxy", err.Error())
	case errors.Is(err, proxyapp.ErrNotFound):
		response.Error(c, http.StatusNotFound, "proxyNotFound", err.Error())
	case errors.Is(err, proxyapp.ErrInUse):
		response.Error(c, http.StatusConflict, "proxyInUse", "代理已被逻辑账号组使用，请先解除绑定")
	default:
		response.Error(c, http.StatusInternalServerError, "proxyOperationFailed", "代理操作失败")
	}
}
