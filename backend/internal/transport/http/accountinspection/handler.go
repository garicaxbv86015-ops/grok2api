// Package accountinspection 提供账号巡检管理接口。
package accountinspection

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	inspectionapp "github.com/chenyme/grok2api/backend/internal/application/accountinspection"
	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/shared/response"
	"github.com/gin-gonic/gin"
)

// Handler 处理账号巡检的管理员 HTTP 请求。
type Handler struct {
	service *inspectionapp.Service
}

// inspectionRequest 描述管理端发起 Grok Build 巡检时可提交的范围参数。
type inspectionRequest struct {
	Provider        string   `json:"provider" binding:"required"`
	IDs             []string `json:"ids"`
	IncludeDisabled bool     `json:"includeDisabled"`
	Concurrency     int      `json:"concurrency"`
	Mode            string   `json:"mode"`
}

// inspectionErrorEvent 是 SSE 失败事件的无敏感信息响应体。
type inspectionErrorEvent struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewHandler 创建账号巡检接口处理器。
// 参数 service 为账号巡检应用服务；返回 HTTP 处理器。
func NewHandler(service *inspectionapp.Service) *Handler {
	return &Handler{service: service}
}

// Register 注册需要管理员认证的 Grok Build 账号巡检路由。
// 参数 router 为已挂载管理员认证中间件的路由分组；无返回值。
func (h *Handler) Register(router *gin.RouterGroup) {
	router.GET("/accounts/inspection", h.overview)
	router.POST("/accounts/inspect", h.inspect)
}

// overview 返回 Build 账号最新巡检快照和分类统计，供独立工作台首次加载与刷新使用。
// 参数 c 为 Gin 请求上下文；响应直接写入上下文，无返回值。
func (h *Handler) overview(c *gin.Context) {
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "accountInspectionUnavailable", "账号巡检服务暂不可用")
		return
	}
	value, err := h.service.GetOverview(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "accountInspectionLoadFailed", "读取账号巡检结果失败")
		return
	}
	response.Success(c, http.StatusOK, value)
}

// inspect 以 SSE 反馈巡检进度，并在完成后发送完整无敏感信息的巡检报告。
// 参数 c 为 Gin 请求上下文；响应直接写入上下文，无返回值。
func (h *Handler) inspect(c *gin.Context) {
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "accountInspectionUnavailable", "账号巡检服务暂不可用")
		return
	}
	input, ok := parseInspectionInput(c)
	if !ok {
		return
	}
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()
	report, err := h.service.Inspect(c.Request.Context(), input, func(progress inspectionapp.Progress) {
		_ = writeInspectionEvent(c, "progress", progress)
	})
	if err != nil {
		_ = writeInspectionEvent(c, "error", inspectionErrorEvent{Code: "accountInspectionFailed", Message: "账号巡检失败"})
		return
	}
	_ = writeInspectionEvent(c, "result", report)
}

// parseInspectionInput 校验 HTTP 请求并转换为应用层巡检输入。
// 参数 c 为 Gin 请求上下文；返回转换后的输入和是否成功。
func parseInspectionInput(c *gin.Context) (inspectionapp.Input, bool) {
	var request inspectionRequest
	if c.ShouldBindJSON(&request) != nil {
		response.Error(c, http.StatusBadRequest, "invalidRequest", "请求参数无效")
		return inspectionapp.Input{}, false
	}
	providerValue := accountdomain.Provider(strings.TrimSpace(request.Provider))
	if providerValue != accountdomain.ProviderBuild {
		response.Error(c, http.StatusBadRequest, "invalidProvider", "仅支持巡检 Grok Build 账号")
		return inspectionapp.Input{}, false
	}
	ids, err := parseInspectionIDs(request.IDs)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalidId", "账号编号无效")
		return inspectionapp.Input{}, false
	}
	return inspectionapp.Input{
		Provider: providerValue, AccountIDs: ids, IncludeDisabled: request.IncludeDisabled,
		Concurrency: request.Concurrency, Mode: inspectionapp.Mode(strings.TrimSpace(request.Mode)),
	}, true
}

// parseInspectionIDs 将字符串账号编号转换为无符号整数，空数组表示巡检全部账号。
// 参数 values 为请求中的账号编号；返回转换后的编号和错误。
func parseInspectionIDs(values []string) ([]uint64, error) {
	ids := make([]uint64, 0, len(values))
	for _, value := range values {
		id, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if err != nil || id == 0 {
			return nil, strconv.ErrSyntax
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// writeInspectionEvent 按 SSE 协议写入一个 JSON 事件并立即刷新到客户端。
// 参数 c 为 Gin 请求上下文，event 为事件名，value 为 JSON 载荷；返回编码或写入错误。
func writeInspectionEvent(c *gin.Context, event string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := c.Writer.WriteString("event: " + event + "\ndata: " + string(data) + "\n\n"); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}
