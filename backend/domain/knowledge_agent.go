package domain

import (
	"errors"
	"time"

	"github.com/chaitin/panda-wiki/pkg/knowledge"
)

const KnowledgeTaskTopic = "apps.panda-wiki.knowledge.task"

var (
	ErrKnowledgeForbidden = errors.New("未获知识管理授权")
	ErrKnowledgeConflict  = errors.New("内容版本已变化，请刷新或重新生成建议")
	ErrKnowledgeInvalid   = errors.New("知识管理参数或模型输出无效")
	ErrKnowledgeSensitive = errors.New("内容可能包含敏感信息，请脱敏后重试")
	ErrKnowledgeBudget    = errors.New("知识管理模型调用预算已耗尽")
	ErrKnowledgeDuplicate = errors.New("该聊天请求已接收，请查看原会话结果")
)

type KnowledgePolicy struct {
	KBID               string    `json:"kb_id" gorm:"primaryKey"`
	Mode               string    `json:"mode"`
	ModelID            string    `json:"model_id"`
	AllowModelTransfer bool      `json:"allow_model_transfer"`
	UserIDs            []string  `json:"user_ids" gorm:"serializer:json;type:jsonb"`
	FolderIDs          []string  `json:"folder_ids" gorm:"serializer:json;type:jsonb"`
	DailyCalls         int       `json:"daily_calls"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (KnowledgePolicy) TableName() string { return "knowledge_agent_policies" }

type KnowledgeConversationState struct {
	ConversationID string    `json:"conversation_id" gorm:"primaryKey"`
	KBID           string    `json:"kb_id"`
	UserID         string    `json:"user_id"`
	Watermark      time.Time `json:"watermark"`
	WatermarkID    string    `json:"watermark_id"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (KnowledgeConversationState) TableName() string { return "knowledge_agent_conversation_states" }

type KnowledgeTask struct {
	ID             string     `json:"id" gorm:"primaryKey"`
	KBID           string     `json:"kb_id"`
	UserID         string     `json:"user_id"`
	ConversationID string     `json:"conversation_id"`
	Kind           string     `json:"kind"`
	Key            string     `json:"key" gorm:"column:task_key"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	AvailableAt    time.Time  `json:"available_at"`
	LeaseUntil     *time.Time `json:"lease_until"`
	LeaseToken     string     `json:"-"`
	MessageIDs     []string   `json:"message_ids" gorm:"serializer:json;type:jsonb"`
	ReleaseIDs     []string   `json:"release_ids" gorm:"serializer:json;type:jsonb"`
	Error          string     `json:"error"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (KnowledgeTask) TableName() string { return "knowledge_agent_tasks" }

type KnowledgeExtraction = knowledge.Extraction

type KnowledgeChange struct {
	ID                string              `json:"id" gorm:"primaryKey"`
	KBID              string              `json:"kb_id"`
	UserID            string              `json:"user_id"`
	TaskID            string              `json:"task_id"`
	Fingerprint       string              `json:"-"`
	ConversationID    string              `json:"conversation_id"`
	ModelID           string              `json:"model_id"`
	Proposal          KnowledgeExtraction `json:"proposal" gorm:"serializer:json;type:jsonb"`
	BaseRevision      int64               `json:"base_revision"`
	PublishedRevision int64               `json:"published_revision"`
	Before            string              `json:"before"`
	After             string              `json:"after"`
	ContentType       string              `json:"content_type"`
	Revision          int64               `json:"revision"`
	Status            string              `json:"status"`
	ReviewerID        string              `json:"reviewer_id"`
	ReviewedAt        *time.Time          `json:"reviewed_at"`
	DocumentID        string              `json:"document_id"`
	ReleaseID         string              `json:"release_id"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

func (KnowledgeChange) TableName() string { return "knowledge_agent_changes" }

type KnowledgeSource struct {
	ID             string `json:"id" gorm:"primaryKey"`
	ChangeID       string `json:"change_id"`
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	UserID         string `json:"user_id"`
}

func (KnowledgeSource) TableName() string { return "knowledge_agent_sources" }

type KnowledgeAudit struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	KBID      string    `json:"kb_id"`
	UserID    string    `json:"user_id"`
	ChangeID  string    `json:"change_id"`
	Action    string    `json:"action"`
	Result    string    `json:"result"`
	CreatedAt time.Time `json:"created_at"`
}

func (KnowledgeAudit) TableName() string { return "knowledge_agent_audits" }

type KnowledgeReviewReq struct {
	KBID             string  `json:"kb_id"`
	ID               string  `json:"id"`
	Revision         int64   `json:"revision"`
	IdempotencyKey   string  `json:"idempotency_key"`
	After            *string `json:"after,omitempty"`
	Title            *string `json:"title,omitempty"`
	NavID            *string `json:"nav_id,omitempty"`
	ParentID         *string `json:"parent_id,omitempty"`
	TargetDocumentID *string `json:"target_document_id,omitempty"`
}

type KnowledgeChatReq struct {
	KBID           string `json:"kb_id"`
	ConversationID string `json:"conversation_id"`
	Message        string `json:"message"`
	IdempotencyKey string `json:"idempotency_key"`
}
