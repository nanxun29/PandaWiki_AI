package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/chaitin/panda-wiki/domain"
	"github.com/chaitin/panda-wiki/repo/pg"
)

// Regenerate reuses the frozen source range; it never rewinds the live message watermark.
func (u *KnowledgeAgentUsecase) Regenerate(ctx context.Context, req domain.KnowledgeReviewReq, userID string) (*domain.KnowledgeTask, error) {
	if err := u.Check(ctx, req.KBID, userID); err != nil {
		return nil, err
	}
	if len(req.IdempotencyKey) == 0 || len(req.IdempotencyKey) > 128 {
		return nil, domain.ErrKnowledgeInvalid
	}
	var result *domain.KnowledgeTask
	err := u.repo.Transaction(ctx, func(r *pg.KnowledgeAgentRepository) error {
		c, err := r.Change(ctx, req.KBID, req.ID, true)
		if err != nil {
			return err
		}
		key := "regenerate:" + c.ID + ":" + req.IdempotencyKey
		if old, err := r.TaskByKey(ctx, key); err == nil {
			if old.UserID != userID {
				return domain.ErrKnowledgeForbidden
			}
			result = old
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if c.Status != "pending" || c.Revision != req.Revision {
			return domain.ErrKnowledgeConflict
		}
		// Raw private sources are used only by their original owner.
		if c.UserID != userID {
			return domain.ErrKnowledgeForbidden
		}
		if _, err := u.authorize(ctx, r, c.KBID, c.UserID); err != nil {
			return err
		}
		original, err := r.Task(ctx, c.KBID, c.TaskID, false)
		if err != nil {
			return err
		}
		t := &domain.KnowledgeTask{ID: uuid.NewString(), KBID: c.KBID, UserID: c.UserID, ConversationID: c.ConversationID, Kind: "extract", Key: key, Status: "pending", MessageIDs: original.MessageIDs, ReleaseIDs: []string{}, AvailableAt: time.Now()}
		if err := r.Enqueue(ctx, t); err != nil {
			return err
		}
		stored, err := r.TaskByKey(ctx, t.Key)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrKnowledgeConflict
			}
			return err
		}
		c.Status = "rejected"
		c.Revision++
		c.ReviewerID = userID
		if err := r.SaveChange(ctx, c); err != nil {
			return err
		}
		if err := r.Audit(ctx, c.KBID, userID, c.ID, "regenerate", "queued"); err != nil {
			return err
		}
		result = stored
		return nil
	})
	return result, err
}

// Rollback proposes a reverse content change. First-version creation is never automatically deleted.
func (u *KnowledgeAgentUsecase) Rollback(ctx context.Context, req domain.KnowledgeReviewReq, userID string) (*domain.KnowledgeChange, error) {
	if err := u.Check(ctx, req.KBID, userID); err != nil {
		return nil, err
	}
	if len(req.IdempotencyKey) == 0 || len(req.IdempotencyKey) > 128 {
		return nil, domain.ErrKnowledgeInvalid
	}
	var result *domain.KnowledgeChange
	err := u.repo.Transaction(ctx, func(r *pg.KnowledgeAgentRepository) error {
		c, err := r.Change(ctx, req.KBID, req.ID, true)
		if err != nil {
			return err
		}
		key := "rollback:" + c.ID + ":" + req.IdempotencyKey
		if old, err := r.TaskByKey(ctx, key); err == nil {
			result, err = r.ChangeByTask(ctx, c.KBID, old.ID)
			return err
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if c.Status != "published" || c.Proposal.Action == "CREATE" {
			return domain.ErrKnowledgeInvalid
		}
		p, err := u.authorize(ctx, r, c.KBID, c.UserID)
		if err != nil {
			return err
		}
		n, err := r.LockNode(ctx, c.KBID, c.DocumentID)
		if err != nil {
			return err
		}
		if n.Revision != c.PublishedRevision || n.Content != c.After {
			return domain.ErrKnowledgeConflict
		}
		if err := u.validateDestination(ctx, p, c.Proposal); err != nil {
			return err
		}
		task := &domain.KnowledgeTask{ID: uuid.NewString(), KBID: c.KBID, UserID: c.UserID, ConversationID: c.ConversationID, Kind: "extract", Key: key, Status: "done", AvailableAt: time.Now(), MessageIDs: c.Proposal.SourceMessageIDs, ReleaseIDs: []string{}}
		if err := r.Enqueue(ctx, task); err != nil {
			return err
		}
		stored, err := r.TaskByKey(ctx, task.Key)
		if err != nil {
			return err
		}
		proposal := c.Proposal
		proposal.Action = "UPDATE"
		proposal.TargetDocumentID = n.ID
		proposal.RiskLevel = "high"
		proposal.Reason = "回滚 AI 变更 " + c.ID
		inverse := &domain.KnowledgeChange{ID: uuid.NewString(), KBID: c.KBID, UserID: c.UserID, TaskID: stored.ID, ConversationID: c.ConversationID, ModelID: c.ModelID, Proposal: proposal, BaseRevision: n.Revision, Before: n.Content, After: c.Before, ContentType: c.ContentType, Revision: 1, Status: "pending"}
		if err := r.SaveChange(ctx, inverse); err != nil {
			return err
		}
		for _, id := range proposal.SourceMessageIDs {
			if err := r.AddSource(ctx, &domain.KnowledgeSource{ID: uuid.NewString(), ChangeID: inverse.ID, ConversationID: c.ConversationID, MessageID: id, UserID: c.UserID}); err != nil {
				return err
			}
		}
		if err := r.Audit(ctx, c.KBID, userID, inverse.ID, "rollback_proposal", "pending_review"); err != nil {
			return err
		}
		result = inverse
		return nil
	})
	return result, err
}
