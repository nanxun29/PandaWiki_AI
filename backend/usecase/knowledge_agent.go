package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/chaitin/panda-wiki/consts"
	"github.com/chaitin/panda-wiki/domain"
	"github.com/chaitin/panda-wiki/log"
	"github.com/chaitin/panda-wiki/pkg/knowledge"
	"github.com/chaitin/panda-wiki/repo/pg"
)

type KnowledgeAgentUsecase struct {
	repo   *pg.KnowledgeAgentRepository
	node   *NodeUsecase
	kb     *KnowledgeBaseUsecase
	models *ModelUsecase
	llm    *LLMUsecase
	nav    *pg.NavRepository
	logger *log.Logger
}

func NewKnowledgeAgentUsecase(repo *pg.KnowledgeAgentRepository, node *NodeUsecase, kb *KnowledgeBaseUsecase, models *ModelUsecase, llm *LLMUsecase, nav *pg.NavRepository, logger *log.Logger) *KnowledgeAgentUsecase {
	return &KnowledgeAgentUsecase{repo: repo, node: node, kb: kb, models: models, llm: llm, nav: nav, logger: logger}
}

func (u *KnowledgeAgentUsecase) Check(ctx context.Context, kbID, userID string) error {
	return u.repo.CheckPermission(ctx, kbID, userID, consts.UserKBPermissionDocManage)
}

func (u *KnowledgeAgentUsecase) Policy(ctx context.Context, kbID, userID string) (*domain.KnowledgePolicy, error) {
	if err := u.Check(ctx, kbID, userID); err != nil {
		return nil, err
	}
	return u.repo.Policy(ctx, kbID, false)
}

func (u *KnowledgeAgentUsecase) SavePolicy(ctx context.Context, p *domain.KnowledgePolicy, userID string) error {
	if err := u.repo.CheckPermission(ctx, p.KBID, userID, consts.UserKBPermissionFullControl); err != nil {
		return err
	}
	if !slices.Contains([]string{"off", "manual", "draft"}, p.Mode) || p.DailyCalls < 1 || p.DailyCalls > 10000 {
		return domain.ErrKnowledgeInvalid
	}
	if p.Mode != "off" {
		if !p.AllowModelTransfer || len(p.UserIDs) == 0 {
			return domain.ErrKnowledgeInvalid
		}
		if p.ModelID != "" {
			configured, err := u.models.modelRepo.GetModelByID(ctx, p.ModelID)
			if err != nil || configured.Type != domain.ModelTypeChat {
				return domain.ErrKnowledgeInvalid
			}
		} else if _, err := u.models.GetChatModel(ctx); err != nil {
			return err
		}
		for _, id := range p.UserIDs {
			if err := u.Check(ctx, p.KBID, id); err != nil {
				return err
			}
		}
		for _, id := range p.FolderIDs {
			if id == "" || id == "*" {
				continue
			}
			n, err := u.repo.Nodes().GetNodeByID(ctx, id)
			if err != nil || n.KBID != p.KBID || n.Type != domain.NodeTypeFolder {
				return domain.ErrKnowledgeInvalid
			}
		}
	}
	return u.repo.Transaction(ctx, func(r *pg.KnowledgeAgentRepository) error {
		if err := r.CheckPermission(ctx, p.KBID, userID, consts.UserKBPermissionFullControl); err != nil {
			return err
		}
		if err := r.SavePolicy(ctx, p); err != nil {
			return err
		}
		return r.Audit(ctx, p.KBID, userID, "", "policy_update", p.Mode)
	})
}

func (u *KnowledgeAgentUsecase) authorize(ctx context.Context, r *pg.KnowledgeAgentRepository, kbID, userID string) (*domain.KnowledgePolicy, error) {
	if strings.EqualFold(os.Getenv("READONLY"), "true") || os.Getenv("READONLY") == "1" {
		return nil, domain.ErrKnowledgeForbidden
	}
	if err := r.CheckPermission(ctx, kbID, userID, consts.UserKBPermissionDocManage); err != nil {
		return nil, err
	}
	p, err := r.Policy(ctx, kbID, true)
	if err != nil {
		return nil, err
	}
	if p.Mode == "off" || !p.AllowModelTransfer || !slices.Contains(p.UserIDs, userID) {
		return nil, domain.ErrKnowledgeForbidden
	}
	return p, nil
}

func (u *KnowledgeAgentUsecase) PrepareChat(ctx context.Context, req *domain.ChatRequest, userID string) error {
	if err := u.Check(ctx, req.KBID, userID); err != nil {
		return err
	}
	if knowledge.Sensitive(req.Message) || len(req.Message) > 32000 || strings.TrimSpace(req.Message) == "" {
		return domain.ErrKnowledgeSensitive
	}
	if req.ConversationID != "" {
		conversation, err := u.repo.Conversation(ctx, req.KBID, req.ConversationID, userID)
		if err != nil {
			return err
		}
		req.Nonce = conversation.Nonce
	}
	req.AdminUserID = userID
	if len(req.KnowledgeRequestKey) == 0 || len(req.KnowledgeRequestKey) > 128 {
		return domain.ErrKnowledgeInvalid
	}
	task, err := u.repo.ReserveChat(ctx, req.KBID, userID, req.KnowledgeRequestKey)
	if err != nil {
		return err
	}
	req.KnowledgeChatTask = task
	return nil
}

func (u *KnowledgeAgentUsecase) FinishChat(ctx context.Context, req *domain.ChatRequest) {
	if req.KnowledgeChatTask == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := u.repo.FinishTask(ctx, req.KnowledgeChatTask, "done", ""); err != nil {
		u.logger.Error("finish chat request ledger failed")
	}
}

func (u *KnowledgeAgentUsecase) RegisterConversation(ctx context.Context, req *domain.ChatRequest) error {
	return u.repo.RegisterConversation(ctx, req.KBID, req.ConversationID, req.AdminUserID)
}

func (u *KnowledgeAgentUsecase) Messages(ctx context.Context, kbID, conversationID, userID string) ([]domain.ConversationMessage, error) {
	if err := u.Check(ctx, kbID, userID); err != nil {
		return nil, err
	}
	if _, err := u.repo.State(ctx, kbID, conversationID, userID); err != nil {
		return nil, err
	}
	return u.repo.Messages(ctx, kbID, conversationID, nil)
}

func (u *KnowledgeAgentUsecase) QueueExtraction(ctx context.Context, kbID, conversationID, userID, key string, manual bool) (*domain.KnowledgeTask, error) {
	p, err := u.authorize(ctx, u.repo, kbID, userID)
	if err != nil {
		return nil, err
	}
	if !manual && p.Mode != "draft" {
		return nil, nil
	}
	state, err := u.repo.State(ctx, kbID, conversationID, userID)
	if err != nil {
		return nil, err
	}
	msgs, err := u.repo.IncrementalMessages(ctx, state)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, nil
	}
	complete := 0
	for _, m := range msgs {
		if m.Role == schema.Assistant {
			complete++
		}
	}
	if complete == 0 {
		return nil, nil
	}
	if len(key) == 0 || len(key) > 128 {
		return nil, domain.ErrKnowledgeInvalid
	}
	t := &domain.KnowledgeTask{ID: uuid.NewString(), KBID: kbID, UserID: userID, ConversationID: conversationID, Kind: "extract", Key: kbID + ":" + userID + ":" + key, Status: "pending", AvailableAt: time.Now(), MessageIDs: []string{}, ReleaseIDs: []string{}}
	if !manual && complete < 3 {
		t.AvailableAt = msgs[len(msgs)-1].CreatedAt.Add(5 * time.Minute)
	}
	if err := u.repo.Enqueue(ctx, t); err != nil {
		return nil, err
	}
	stored, err := u.repo.TaskByKey(ctx, t.Key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		active, err := u.repo.ActiveTask(ctx, kbID, conversationID, userID)
		if err != nil {
			return nil, err
		}
		if err := u.repo.ReschedulePending(ctx, active.ID, t.AvailableAt); err != nil {
			return nil, err
		}
		active.AvailableAt = t.AvailableAt
		return active, nil
	}
	return stored, err
}

func (u *KnowledgeAgentUsecase) model(ctx context.Context, p *domain.KnowledgePolicy, userID string) (model.BaseChatModel, *domain.Model, error) {
	fresh, err := u.authorize(ctx, u.repo, p.KBID, userID)
	if err != nil {
		return nil, nil, err
	}
	p = fresh
	if err := u.repo.ConsumeBudget(ctx, p.KBID, userID, p.DailyCalls); err != nil {
		return nil, nil, err
	}
	var configured *domain.Model
	if p.ModelID == "" {
		configured, err = u.models.GetChatModel(ctx)
	} else {
		configured, err = u.models.modelRepo.GetModelByID(ctx, p.ModelID)
	}
	if err != nil {
		return nil, nil, err
	}
	m, err := configured.ToModelkitModel()
	if err != nil {
		return nil, nil, err
	}
	chat, err := u.llm.modelkit.GetChatModel(ctx, m)
	return chat, configured, err
}

func (u *KnowledgeAgentUsecase) generate(ctx context.Context, p *domain.KnowledgePolicy, userID, system string, input any, target any) (string, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	if len(encoded) > 256*1024 {
		return "", domain.ErrKnowledgeInvalid
	}
	if knowledge.Sensitive(string(encoded)) {
		return "", domain.ErrKnowledgeSensitive
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	m, configured, err := u.model(ctx, p, userID)
	if err != nil {
		return "", err
	}
	resp, err := m.Generate(ctx, []*schema.Message{schema.SystemMessage(system), schema.UserMessage(string(encoded))})
	if err != nil {
		return "", fmt.Errorf("knowledge model: %w", err)
	}
	if resp.ResponseMeta != nil && resp.ResponseMeta.Usage != nil {
		if err := u.models.UpdateUsage(ctx, configured.ID, resp.ResponseMeta.Usage); err != nil {
			return "", err
		}
	}
	if err := knowledge.Decode(resp.Content, target); err != nil {
		return "", domain.ErrKnowledgeInvalid
	}
	identity := configured.ID
	if identity == "" {
		identity = string(configured.Provider) + "/" + configured.Model
	}
	return identity, nil
}

func (u *KnowledgeAgentUsecase) Similar(ctx context.Context, kbID, userID, query string) ([]*domain.Node, error) {
	if err := u.Check(ctx, kbID, userID); err != nil {
		return nil, err
	}
	if knowledge.Sensitive(query) || len(query) > 8000 {
		return nil, domain.ErrKnowledgeSensitive
	}
	kb, err := u.kb.repo.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		return nil, err
	}
	_, ranked, err := u.llm.GetRankNodes(ctx, GetRankNodesRequest{DatasetID: kb.DatasetID, Question: query, SimilarityThreshold: 0.2})
	if err != nil {
		return nil, fmt.Errorf("similarity search unavailable: %w", err)
	}
	nodes := []*domain.Node{}
	seen := map[string]bool{}
	add := func(id string) error {
		if seen[id] {
			return nil
		}
		seen[id] = true
		n, err := u.repo.Nodes().GetNodeByID(ctx, id)
		if err != nil {
			return err
		}
		if n.KBID != kbID {
			return domain.ErrKnowledgeForbidden
		}
		if n.Type == domain.NodeTypeDocument {
			nodes = append(nodes, n)
		}
		return nil
	}
	for _, n := range ranked {
		if err := add(n.NodeID); err != nil {
			return nil, err
		}
	}
	titles, err := u.node.GetList(ctx, &domain.GetNodeListReq{KBID: kbID, Search: query})
	if err != nil {
		return nil, err
	}
	for _, n := range titles {
		if len(nodes) >= 10 {
			break
		}
		if err := add(n.ID); err != nil {
			return nil, err
		}
	}
	return nodes, nil
}

func (u *KnowledgeAgentUsecase) validateDestination(ctx context.Context, p *domain.KnowledgePolicy, e domain.KnowledgeExtraction) error {
	if !slices.Contains(p.FolderIDs, "*") && !slices.Contains(p.FolderIDs, e.ParentID) {
		return domain.ErrKnowledgeForbidden
	}
	nav, err := u.nav.GetById(ctx, e.NavID)
	if err != nil || nav.KbID != p.KBID {
		return domain.ErrKnowledgeInvalid
	}
	if e.ParentID != "" {
		n, err := u.repo.Nodes().GetNodeByID(ctx, e.ParentID)
		if err != nil || n.KBID != p.KBID || n.NavId != e.NavID || n.Type != domain.NodeTypeFolder {
			return domain.ErrKnowledgeInvalid
		}
	}
	return nil
}

func (u *KnowledgeAgentUsecase) buildChange(ctx context.Context, t *domain.KnowledgeTask, e domain.KnowledgeExtraction, modelID string) (*domain.KnowledgeChange, error) {
	if err := knowledge.Validate(e); err != nil {
		return nil, domain.ErrKnowledgeInvalid
	}
	p, err := u.authorize(ctx, u.repo, t.KBID, t.UserID)
	if err != nil {
		return nil, err
	}
	msgs, err := u.repo.Messages(ctx, t.KBID, t.ConversationID, t.MessageIDs)
	if err != nil {
		return nil, err
	}
	allowed := map[string]domain.ConversationMessage{}
	for _, m := range msgs {
		allowed[m.ID] = m
	}
	userEvidence := false
	for _, id := range e.SourceMessageIDs {
		m, ok := allowed[id]
		if !ok {
			return nil, domain.ErrKnowledgeInvalid
		}
		if m.Role == schema.User {
			userEvidence = true
		}
	}
	if !userEvidence {
		return nil, domain.ErrKnowledgeInvalid
	}
	change := &domain.KnowledgeChange{ID: uuid.NewString(), KBID: t.KBID, UserID: t.UserID, TaskID: t.ID, ConversationID: t.ConversationID, ModelID: modelID, Proposal: e, Revision: 1, Status: "pending", ContentType: "md"}
	if e.Action != "CREATE" {
		n, err := u.repo.Nodes().GetNodeByID(ctx, e.TargetDocumentID)
		if err != nil || n.KBID != t.KBID || n.Type != domain.NodeTypeDocument {
			return nil, domain.ErrKnowledgeInvalid
		}
		change.BaseRevision = n.Revision
		change.Before = n.Content
		change.ContentType = n.Meta.ContentType
		e.NavID = n.NavId
		e.ParentID = n.ParentID
	}
	if err := u.validateDestination(ctx, p, e); err != nil {
		return nil, err
	}
	change.Proposal = e
	change.After, err = knowledge.Apply(change.Before, change.ContentType, e)
	if err != nil {
		change.After = change.Before
		change.Proposal.RiskLevel = "high"
		change.Proposal.Reason += "；无法安全自动应用，请人工编辑正文"
	}
	if knowledge.Sensitive(change.Before + change.After) {
		return nil, domain.ErrKnowledgeSensitive
	}
	// A model-supplied low risk label cannot downgrade destructive or unverified changes.
	if e.Action == "UPDATE" || e.Action == "MERGE" || e.Action == "CONFLICT" || e.VerificationStatus == "ai_suggested" || e.VerificationStatus == "unverified" ||
		(e.Action == "APPEND" && len(change.After)-len(change.Before) > len(change.Before)/4) {
		change.Proposal.RiskLevel = "high"
	}
	change.Fingerprint = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%s:%s", e.Action, e.TargetDocumentID, change.BaseRevision, e.Title, change.After))))
	return change, nil
}

func (u *KnowledgeAgentUsecase) Changes(ctx context.Context, kbID, userID, status string) ([]domain.KnowledgeChange, error) {
	if err := u.Check(ctx, kbID, userID); err != nil {
		return nil, err
	}
	return u.repo.Changes(ctx, kbID, status)
}

func (u *KnowledgeAgentUsecase) Tasks(ctx context.Context, kbID, userID string) ([]domain.KnowledgeTask, error) {
	if err := u.Check(ctx, kbID, userID); err != nil {
		return nil, err
	}
	return u.repo.ListTasks(ctx, kbID, userID)
}

func (u *KnowledgeAgentUsecase) TaskAction(ctx context.Context, kbID, id, userID, action string) error {
	if err := u.Check(ctx, kbID, userID); err != nil {
		return err
	}
	t, err := u.repo.Task(ctx, kbID, id, false)
	if err != nil {
		return err
	}
	if t.Kind == "chat" || (t.Kind == "index" && action == "cancel") {
		return domain.ErrKnowledgeInvalid
	}
	if t.UserID != userID {
		return domain.ErrKnowledgeForbidden
	}
	if t.Kind == "extract" {
		if _, err := u.authorize(ctx, u.repo, kbID, userID); err != nil {
			return err
		}
	}
	status := "cancelled"
	if action == "retry" {
		status = "pending"
	} else if action != "cancel" {
		return domain.ErrKnowledgeInvalid
	}
	return u.repo.SetTaskStatus(ctx, kbID, id, userID, status)
}

func (u *KnowledgeAgentUsecase) Audits(ctx context.Context, kbID, userID string) ([]domain.KnowledgeAudit, error) {
	if err := u.Check(ctx, kbID, userID); err != nil {
		return nil, err
	}
	return u.repo.Audits(ctx, kbID)
}

func (u *KnowledgeAgentUsecase) Review(ctx context.Context, req domain.KnowledgeReviewReq, userID, action string) (*domain.KnowledgeChange, error) {
	if len(req.IdempotencyKey) == 0 || len(req.IdempotencyKey) > 128 {
		return nil, domain.ErrKnowledgeInvalid
	}
	if err := u.Check(ctx, req.KBID, userID); err != nil {
		return nil, err
	}
	var result *domain.KnowledgeChange
	err := u.repo.Transaction(ctx, func(r *pg.KnowledgeAgentRepository) error {
		c, err := r.Change(ctx, req.KBID, req.ID, true)
		if err != nil {
			return err
		}
		if (action == "approve" && c.Status == "published") || (action == "reject" && c.Status == "rejected") {
			result = c
			return nil
		}
		if c.Status != "pending" || c.Revision != req.Revision {
			return domain.ErrKnowledgeConflict
		}
		if err := r.CheckPermission(ctx, req.KBID, userID, consts.UserKBPermissionDocManage); err != nil {
			return err
		}
		switch action {
		case "reject":
			c.Status = "rejected"
		case "edit", "approve":
			targetChanged := false
			p, err := u.authorize(ctx, r, c.KBID, c.UserID)
			if err != nil {
				return err
			}
			if action == "edit" {
				if req.Title != nil {
					if len(strings.TrimSpace(*req.Title)) == 0 || len(*req.Title) > 500 || knowledge.Sensitive(*req.Title) {
						return domain.ErrKnowledgeInvalid
					}
					c.Proposal.Title = *req.Title
				}
				if req.TargetDocumentID != nil && *req.TargetDocumentID != c.Proposal.TargetDocumentID {
					targetChanged = true
					if *req.TargetDocumentID == "" {
						c.Proposal.Action = "CREATE"
						c.Before = ""
						c.After = c.Proposal.Content
						c.BaseRevision = 0
						c.ContentType = "md"
					} else {
						n, err := r.LockNode(ctx, c.KBID, *req.TargetDocumentID)
						if err != nil || n.Type != domain.NodeTypeDocument {
							return domain.ErrKnowledgeInvalid
						}
						c.Before = n.Content
						c.After = n.Content
						c.BaseRevision = n.Revision
						c.ContentType = n.Meta.ContentType
						c.Proposal.NavID = n.NavId
						c.Proposal.ParentID = n.ParentID
						c.Proposal.Action = "UPDATE"
					}
					c.Proposal.TargetDocumentID = *req.TargetDocumentID
				}
				if c.Proposal.Action == "CREATE" {
					if req.NavID != nil {
						c.Proposal.NavID = *req.NavID
					}
					if req.ParentID != nil {
						c.Proposal.ParentID = *req.ParentID
					}
				}
			}
			if err := u.validateDestination(ctx, p, c.Proposal); err != nil {
				return err
			}
			if req.After != nil && !targetChanged {
				if len(*req.After) > 256*1024 || knowledge.Sensitive(*req.After) {
					return domain.ErrKnowledgeSensitive
				}
				c.After = *req.After
			}
			if action == "approve" {
				if strings.TrimSpace(c.After) == "" || c.After == c.Before {
					return domain.ErrKnowledgeInvalid
				}
				if c.Proposal.Action == "MERGE" || c.Proposal.Action == "CONFLICT" {
					if req.After == nil {
						return domain.ErrKnowledgeInvalid
					}
				}
				if err := u.publish(ctx, r, c, userID); err != nil {
					return err
				}
				c.Status = "published"
			}
		default:
			return domain.ErrKnowledgeInvalid
		}
		c.Revision++
		now := time.Now()
		c.ReviewerID = userID
		c.ReviewedAt = &now
		if err := r.SaveChange(ctx, c); err != nil {
			return err
		}
		if err := r.Audit(ctx, c.KBID, userID, c.ID, action, "success"); err != nil {
			return err
		}
		result = c
		return nil
	})
	return result, err
}

func (u *KnowledgeAgentUsecase) publish(ctx context.Context, r *pg.KnowledgeAgentRepository, c *domain.KnowledgeChange, userID string) error {
	nodeUC := *u.node
	nodeUC.nodeRepo = r.Nodes()
	if c.Proposal.Action == "CREATE" {
		format := "md"
		id, err := nodeUC.Create(ctx, &domain.CreateNodeReq{KBID: c.KBID, NavId: c.Proposal.NavID, ParentID: c.Proposal.ParentID, Type: domain.NodeTypeDocument, Name: c.Proposal.Title, Content: c.After, ContentType: &format, MaxNode: domain.GetBaseEditionLimitation(ctx).MaxNode}, userID)
		if err != nil {
			return err
		}
		c.DocumentID = id
	} else {
		n, err := r.LockNode(ctx, c.KBID, c.Proposal.TargetDocumentID)
		if err != nil {
			return err
		}
		if n.Revision != c.BaseRevision || n.Content != c.Before || n.NavId != c.Proposal.NavID || n.ParentID != c.Proposal.ParentID {
			return domain.ErrKnowledgeConflict
		}
		if err := nodeUC.Update(ctx, &domain.UpdateNodeReq{ID: n.ID, KBID: c.KBID, Content: &c.After}, userID); err != nil {
			return err
		}
		c.DocumentID = n.ID
	}
	kbUC := *u.kb
	kbUC.repo = r.BindKB(u.kb.repo)
	kbUC.nodeRepo = r.Nodes()
	id, err := kbUC.createKBRelease(ctx, &domain.CreateKBReleaseReq{KBID: c.KBID, NodeIDs: []string{c.DocumentID}, Message: "AI 知识变更 " + c.ID}, userID, func(ctx context.Context, requests []*domain.NodeReleaseVectorRequest) error {
		ids := []string{}
		for _, req := range requests {
			ids = append(ids, req.NodeReleaseID)
		}
		return r.Enqueue(ctx, &domain.KnowledgeTask{ID: uuid.NewString(), KBID: c.KBID, UserID: userID, Kind: "index", Key: "index:" + c.ID, Status: "pending", AvailableAt: time.Now(), MessageIDs: []string{}, ReleaseIDs: ids})
	})
	if err != nil {
		return err
	}
	c.ReleaseID = id
	n, err := r.Nodes().GetNodeByID(ctx, c.DocumentID)
	if err != nil {
		return err
	}
	c.PublishedRevision = n.Revision
	return nil
}
