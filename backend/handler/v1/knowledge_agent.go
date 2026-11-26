package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/gorm"

	"github.com/chaitin/panda-wiki/domain"
	"github.com/chaitin/panda-wiki/handler"
	pwMiddleware "github.com/chaitin/panda-wiki/middleware"
	"github.com/chaitin/panda-wiki/usecase"
)

type KnowledgeAgentHandler struct {
	*handler.BaseHandler
	agent *usecase.KnowledgeAgentUsecase
	chat  *usecase.ChatUsecase
}

func NewKnowledgeAgentHandler(e *echo.Echo, base *handler.BaseHandler, auth pwMiddleware.AuthMiddleware, agent *usecase.KnowledgeAgentUsecase, chat *usecase.ChatUsecase) *KnowledgeAgentHandler {
	h := &KnowledgeAgentHandler{BaseHandler: base, agent: agent, chat: chat}
	g := e.Group("/api/v1/knowledge-agent", auth.Authorize, middleware.BodyLimit("1M"), func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			info := domain.GetAuthInfoFromCtx(c.Request().Context())
			if info == nil || info.IsToken {
				return h.respond(c, nil, domain.ErrKnowledgeForbidden)
			}
			return next(c)
		}
	})
	g.GET("/policy", h.GetPolicy)
	g.PUT("/policy", h.PutPolicy)
	g.POST("/chat", h.Chat)
	g.GET("/messages", h.Messages)
	g.POST("/extract", h.Extract)
	g.GET("/changes", h.Changes)
	g.POST("/review/:action", h.Review)
	g.POST("/changes/regenerate", h.Regenerate)
	g.POST("/changes/rollback", h.Rollback)
	g.GET("/tasks", h.Tasks)
	g.POST("/tasks/:action", h.TaskAction)
	g.GET("/audits", h.Audits)
	return h
}

func (h *KnowledgeAgentHandler) user(c echo.Context) string {
	return domain.GetAuthInfoFromCtx(c.Request().Context()).UserId
}

func (h *KnowledgeAgentHandler) respond(c echo.Context, data any, err error) error {
	if err == nil {
		return h.NewResponseWithData(c, data)
	}
	status := http.StatusInternalServerError
	message := "知识管理操作失败，请稍后重试"
	switch {
	case errors.Is(err, domain.ErrKnowledgeForbidden):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, domain.ErrKnowledgeConflict), errors.Is(err, domain.ErrKnowledgeDuplicate):
		status = http.StatusConflict
		message = err.Error()
	case errors.Is(err, domain.ErrKnowledgeInvalid), errors.Is(err, domain.ErrKnowledgeSensitive):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, domain.ErrKnowledgeBudget):
		status = http.StatusTooManyRequests
		message = err.Error()
	case errors.Is(err, gorm.ErrRecordNotFound):
		status = http.StatusNotFound
		message = "记录不存在"
	}
	return c.JSON(status, domain.PWResponse{Success: false, Message: message, Code: status})
}

// GetPolicy returns the current knowledge management policy.
//
//	@Summary	获取知识管理策略
//	@Tags		knowledge_agent
//	@Security	bearerAuth
//	@Param		kb_id	query		string	true	"知识库 ID"
//	@Success	200		{object}	domain.PWResponse
//	@Router		/api/v1/knowledge-agent/policy [get]
func (h *KnowledgeAgentHandler) GetPolicy(c echo.Context) error {
	p, err := h.agent.Policy(c.Request().Context(), c.QueryParam("kb_id"), h.user(c))
	return h.respond(c, map[string]any{"policy": p, "current_user_id": h.user(c)}, err)
}

// PutPolicy updates a policy using the current full-control user's identity.
//
//	@Summary	更新知识管理策略
//	@Tags		knowledge_agent
//	@Security	bearerAuth
//	@Param		body	body		domain.KnowledgePolicy	true	"策略"
//	@Success	200		{object}	domain.PWResponse
//	@Router		/api/v1/knowledge-agent/policy [put]
func (h *KnowledgeAgentHandler) PutPolicy(c echo.Context) error {
	var p domain.KnowledgePolicy
	if err := c.Bind(&p); err != nil {
		return h.respond(c, nil, domain.ErrKnowledgeInvalid)
	}
	err := h.agent.SavePolicy(c.Request().Context(), &p, h.user(c))
	return h.respond(c, p, err)
}

// Chat streams the original chat service for a backend-owned conversation.
//
//	@Summary	后台知识助手流式聊天
//	@Tags		knowledge_agent
//	@Security	bearerAuth
//	@Accept		json
//	@Produce	text/event-stream
//	@Param		body	body		domain.KnowledgeChatReq	true	"聊天请求"
//	@Success	200		{string}	string					"SSE"
//	@Router		/api/v1/knowledge-agent/chat [post]
func (h *KnowledgeAgentHandler) Chat(c echo.Context) error {
	var body domain.KnowledgeChatReq
	if err := c.Bind(&body); err != nil || len(body.IdempotencyKey) == 0 || len(body.IdempotencyKey) > 128 {
		return h.respond(c, nil, domain.ErrKnowledgeInvalid)
	}
	req := &domain.ChatRequest{KBID: body.KBID, ConversationID: body.ConversationID, Message: body.Message, AppType: domain.AppTypeWeb, RemoteIP: c.RealIP(), KnowledgeRequestKey: body.IdempotencyKey}
	if err := h.agent.PrepareChat(c.Request().Context(), req, h.user(c)); err != nil {
		return h.respond(c, nil, err)
	}
	ch, err := h.chat.Chat(c.Request().Context(), req)
	if err != nil {
		return h.respond(c, nil, err)
	}
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	write := func(event domain.SSEEvent) error {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Response(), "data: %s\n\n", data); err != nil {
			return err
		}
		c.Response().Flush()
		return nil
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			if err := write(event); err != nil {
				return err
			}
		case <-ticker.C:
			if _, err := fmt.Fprint(c.Response(), ": heartbeat\n\n"); err != nil {
				return err
			}
			c.Response().Flush()
		}
	}
}

// Messages returns only the current backend user's conversation.
//
//	@Summary	获取私有助手会话消息
//	@Tags		knowledge_agent
//	@Security	bearerAuth
//	@Param		kb_id			query		string	true	"知识库 ID"
//	@Param		conversation_id	query		string	true	"会话 ID"
//	@Success	200				{object}	domain.PWResponse{data=[]domain.ConversationMessage}
//	@Router		/api/v1/knowledge-agent/messages [get]
func (h *KnowledgeAgentHandler) Messages(c echo.Context) error {
	data, err := h.agent.Messages(c.Request().Context(), c.QueryParam("kb_id"), c.QueryParam("conversation_id"), h.user(c))
	return h.respond(c, data, err)
}

// Extract schedules manual extraction without waiting for model work.
//
//	@Summary	提交聊天知识整理任务
//	@Tags		knowledge_agent
//	@Security	bearerAuth
//	@Param		body	body		domain.KnowledgeChatReq	true	"会话及幂等键"
//	@Success	200		{object}	domain.PWResponse{data=domain.KnowledgeTask}
//	@Router		/api/v1/knowledge-agent/extract [post]
func (h *KnowledgeAgentHandler) Extract(c echo.Context) error {
	var req domain.KnowledgeChatReq
	if err := c.Bind(&req); err != nil {
		return h.respond(c, nil, domain.ErrKnowledgeInvalid)
	}
	data, err := h.agent.QueueExtraction(c.Request().Context(), req.KBID, req.ConversationID, h.user(c), req.IdempotencyKey, true)
	return h.respond(c, data, err)
}

// Changes lists proposals; unpublished bodies never enter RAG.
//
//	@Summary	获取知识收件箱
//	@Tags		knowledge_agent
//	@Security	bearerAuth
//	@Param		kb_id	query		string	true	"知识库 ID"
//	@Param		status	query		string	false	"状态"
//	@Success	200		{object}	domain.PWResponse{data=[]domain.KnowledgeChange}
//	@Router		/api/v1/knowledge-agent/changes [get]
func (h *KnowledgeAgentHandler) Changes(c echo.Context) error {
	data, err := h.agent.Changes(c.Request().Context(), c.QueryParam("kb_id"), h.user(c), c.QueryParam("status"))
	return h.respond(c, data, err)
}

// Review uses revision checks and the original publication service.
//
//	@Summary	编辑、批准或拒绝知识变更
//	@Tags		knowledge_agent
//	@Security	bearerAuth
//	@Param		action	path		string						true	"edit/approve/reject"
//	@Param		body	body		domain.KnowledgeReviewReq	true	"审核请求"
//	@Success	200		{object}	domain.PWResponse{data=domain.KnowledgeChange}
//	@Router		/api/v1/knowledge-agent/review/{action} [post]
func (h *KnowledgeAgentHandler) Review(c echo.Context) error {
	var req domain.KnowledgeReviewReq
	if err := c.Bind(&req); err != nil {
		return h.respond(c, nil, domain.ErrKnowledgeInvalid)
	}
	data, err := h.agent.Review(c.Request().Context(), req, h.user(c), c.Param("action"))
	return h.respond(c, data, err)
}

// Tasks lists the current user's tasks.
//
//	@Summary	获取知识任务状态
//	@Tags		knowledge_agent
//	@Security	bearerAuth
//	@Param		kb_id	query		string	true	"知识库 ID"
//	@Success	200		{object}	domain.PWResponse{data=[]domain.KnowledgeTask}
//	@Router		/api/v1/knowledge-agent/tasks [get]
func (h *KnowledgeAgentHandler) Tasks(c echo.Context) error {
	data, err := h.agent.Tasks(c.Request().Context(), c.QueryParam("kb_id"), h.user(c))
	return h.respond(c, data, err)
}

// TaskAction retries or cancels an owned extraction task.
//
//	@Summary	重试或取消知识任务
//	@Tags		knowledge_agent
//	@Security	bearerAuth
//	@Param		action	path		string						true	"retry/cancel"
//	@Param		body	body		domain.KnowledgeReviewReq	true	"任务 ID"
//	@Success	200		{object}	domain.PWResponse
//	@Router		/api/v1/knowledge-agent/tasks/{action} [post]
func (h *KnowledgeAgentHandler) TaskAction(c echo.Context) error {
	var req domain.KnowledgeReviewReq
	if err := c.Bind(&req); err != nil {
		return h.respond(c, nil, domain.ErrKnowledgeInvalid)
	}
	err := h.agent.TaskAction(c.Request().Context(), req.KBID, req.ID, h.user(c), strings.ToLower(c.Param("action")))
	return h.respond(c, nil, err)
}

// Audits lists metadata without raw prompts or credentials.
//
//	@Summary	获取 AI 操作审计
//	@Tags		knowledge_agent
//	@Security	bearerAuth
//	@Param		kb_id	query		string	true	"知识库 ID"
//	@Success	200		{object}	domain.PWResponse{data=[]domain.KnowledgeAudit}
//	@Router		/api/v1/knowledge-agent/audits [get]
func (h *KnowledgeAgentHandler) Audits(c echo.Context) error {
	data, err := h.agent.Audits(c.Request().Context(), c.QueryParam("kb_id"), h.user(c))
	return h.respond(c, data, err)
}

// Regenerate re-extracts an owned proposal's source messages.
//
//	@Summary	重新提取知识建议
//	@Tags		knowledge_agent
//	@Security	bearerAuth
//	@Param		body	body		domain.KnowledgeReviewReq	true	"原建议和幂等键"
//	@Success	200		{object}	domain.PWResponse{data=domain.KnowledgeTask}
//	@Router		/api/v1/knowledge-agent/changes/regenerate [post]
func (h *KnowledgeAgentHandler) Regenerate(c echo.Context) error {
	var req domain.KnowledgeReviewReq
	if err := c.Bind(&req); err != nil {
		return h.respond(c, nil, domain.ErrKnowledgeInvalid)
	}
	data, err := h.agent.Regenerate(c.Request().Context(), req, h.user(c))
	return h.respond(c, data, err)
}

// Rollback creates a reviewable reverse change without deleting documents.
//
//	@Summary	创建回滚建议
//	@Tags		knowledge_agent
//	@Security	bearerAuth
//	@Param		body	body		domain.KnowledgeReviewReq	true	"已发布变更和幂等键"
//	@Success	200		{object}	domain.PWResponse{data=domain.KnowledgeChange}
//	@Router		/api/v1/knowledge-agent/changes/rollback [post]
func (h *KnowledgeAgentHandler) Rollback(c echo.Context) error {
	var req domain.KnowledgeReviewReq
	if err := c.Bind(&req); err != nil {
		return h.respond(c, nil, domain.ErrKnowledgeInvalid)
	}
	data, err := h.agent.Rollback(c.Request().Context(), req, h.user(c))
	return h.respond(c, data, err)
}
