package pg

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/chaitin/panda-wiki/consts"
	"github.com/chaitin/panda-wiki/domain"
	"github.com/chaitin/panda-wiki/log"
	store "github.com/chaitin/panda-wiki/store/pg"
)

type KnowledgeAgentRepository struct {
	db     *store.DB
	logger *log.Logger
}

func NewKnowledgeAgentRepository(db *store.DB, logger *log.Logger) *KnowledgeAgentRepository {
	return &KnowledgeAgentRepository{db: db, logger: logger}
}

// Transaction binds existing repositories to the same transaction. Business rules stay in usecase.
func (r *KnowledgeAgentRepository) Transaction(ctx context.Context, fn func(*KnowledgeAgentRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&KnowledgeAgentRepository{db: &store.DB{DB: tx}, logger: r.logger})
	})
}

func (r *KnowledgeAgentRepository) Nodes() *NodeRepository { return NewNodeRepository(r.db, r.logger) }

func (r *KnowledgeAgentRepository) BindKB(repo *KnowledgeBaseRepository) *KnowledgeBaseRepository {
	copyRepo := *repo
	copyRepo.db = r.db
	return &copyRepo
}

func (r *KnowledgeAgentRepository) CheckPermission(ctx context.Context, kbID, userID string, perm consts.UserKBPermission) error {
	if kbID == "" || userID == "" {
		return domain.ErrKnowledgeForbidden
	}
	var user domain.User
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ?", userID).First(&user).Error; err != nil {
		return domain.ErrKnowledgeForbidden
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&domain.KnowledgeBase{}).Where("id = ?", kbID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return domain.ErrKnowledgeForbidden
	}
	if user.Role == consts.UserRoleAdmin {
		return nil
	}
	var membership domain.KBUsers
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where("kb_id = ? AND user_id = ?", kbID, userID).First(&membership).Error; err != nil {
		return domain.ErrKnowledgeForbidden
	}
	if membership.Perm == consts.UserKBPermissionFullControl || membership.Perm == perm {
		return nil
	}
	return domain.ErrKnowledgeForbidden
}

func (r *KnowledgeAgentRepository) Policy(ctx context.Context, kbID string, lock bool) (*domain.KnowledgePolicy, error) {
	p := &domain.KnowledgePolicy{KBID: kbID, Mode: "off", DailyCalls: 50, UserIDs: []string{}, FolderIDs: []string{}}
	q := r.db.WithContext(ctx).Where("kb_id = ?", kbID)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return p, nil
	}
	return p, err
}

func (r *KnowledgeAgentRepository) SavePolicy(ctx context.Context, p *domain.KnowledgePolicy) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "kb_id"}}, UpdateAll: true}).Create(p).Error
}

func (r *KnowledgeAgentRepository) State(ctx context.Context, kbID, conversationID, userID string) (*domain.KnowledgeConversationState, error) {
	var state domain.KnowledgeConversationState
	err := r.db.WithContext(ctx).Where("kb_id = ? AND conversation_id = ? AND user_id = ?", kbID, conversationID, userID).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrKnowledgeForbidden
	}
	return &state, err
}

func (r *KnowledgeAgentRepository) RegisterConversation(ctx context.Context, kbID, conversationID, userID string) error {
	state := domain.KnowledgeConversationState{ConversationID: conversationID, KBID: kbID, UserID: userID, Watermark: time.Unix(0, 0).UTC()}
	if err := r.db.WithContext(ctx).Create(&state).Error; err != nil {
		return err
	}
	return nil
}

func (r *KnowledgeAgentRepository) Conversation(ctx context.Context, kbID, conversationID, userID string) (*domain.Conversation, error) {
	if _, err := r.State(ctx, kbID, conversationID, userID); err != nil {
		return nil, err
	}
	var c domain.Conversation
	err := r.db.WithContext(ctx).Where("id = ? AND kb_id = ?", conversationID, kbID).First(&c).Error
	return &c, err
}

func (r *KnowledgeAgentRepository) IncrementalMessages(ctx context.Context, state *domain.KnowledgeConversationState) ([]domain.ConversationMessage, error) {
	msgs := []domain.ConversationMessage{}
	err := r.db.WithContext(ctx).Where("conversation_id = ? AND kb_id = ?", state.ConversationID, state.KBID).
		Where("created_at > ? OR (created_at = ? AND id > ?)", state.Watermark, state.Watermark, state.WatermarkID).
		Order("created_at, id").Limit(200).Find(&msgs).Error
	return msgs, err
}

func (r *KnowledgeAgentRepository) Messages(ctx context.Context, kbID, conversationID string, ids []string) ([]domain.ConversationMessage, error) {
	msgs := []domain.ConversationMessage{}
	q := r.db.WithContext(ctx).Where("kb_id = ? AND conversation_id = ?", kbID, conversationID)
	if ids != nil {
		q = q.Where("id IN ?", ids)
	}
	err := q.Order("created_at, id").Limit(200).Find(&msgs).Error
	return msgs, err
}

func (r *KnowledgeAgentRepository) Enqueue(ctx context.Context, t *domain.KnowledgeTask) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(t).Error
}

func (r *KnowledgeAgentRepository) Task(ctx context.Context, kbID, id string, lock bool) (*domain.KnowledgeTask, error) {
	var t domain.KnowledgeTask
	q := r.db.WithContext(ctx).Where("kb_id = ? AND id = ?", kbID, id)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(&t).Error
	return &t, err
}

func (r *KnowledgeAgentRepository) TaskByKey(ctx context.Context, key string) (*domain.KnowledgeTask, error) {
	var t domain.KnowledgeTask
	err := r.db.WithContext(ctx).Where("task_key = ?", key).First(&t).Error
	return &t, err
}

func (r *KnowledgeAgentRepository) ActiveTask(ctx context.Context, kbID, conversationID, userID string) (*domain.KnowledgeTask, error) {
	var t domain.KnowledgeTask
	err := r.db.WithContext(ctx).Where("kb_id=? AND conversation_id=? AND user_id=? AND kind='extract' AND status IN ('pending','running','paused')", kbID, conversationID, userID).First(&t).Error
	return &t, err
}

func (r *KnowledgeAgentRepository) DueTasks(ctx context.Context) ([]domain.KnowledgeTask, error) {
	tasks := []domain.KnowledgeTask{}
	err := r.db.WithContext(ctx).Where("kind IN ('extract','index') AND ((status = 'pending' AND available_at <= now()) OR (status = 'running' AND lease_until < now()))").Order("available_at").Limit(100).Find(&tasks).Error
	return tasks, err
}

func (r *KnowledgeAgentRepository) Claim(ctx context.Context, id string) (*domain.KnowledgeTask, error) {
	var task *domain.KnowledgeTask
	err := r.Transaction(ctx, func(tx *KnowledgeAgentRepository) error {
		// A database advisory lock bounds concurrency across all worker instances.
		if err := tx.db.Exec("SELECT pg_advisory_xact_lock(67331039)").Error; err != nil {
			return err
		}
		var active int64
		if err := tx.db.Model(&domain.KnowledgeTask{}).Where("kind IN ('extract','index') AND status='running' AND lease_until > now()").Count(&active).Error; err != nil {
			return err
		}
		if active >= 2 {
			return nil
		}
		var t domain.KnowledgeTask
		err := tx.db.Where("id = ? AND kind IN ('extract','index') AND ((status='pending' AND available_at <= now()) OR (status='running' AND lease_until < now()))", id).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).First(&t).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if t.Attempts >= 3 {
			return tx.db.Model(&t).Updates(map[string]any{"status": "failed", "error": "重试次数已用完"}).Error
		}
		until := time.Now().Add(10 * time.Minute)
		t.Status = "running"
		t.Attempts++
		t.LeaseUntil = &until
		t.LeaseToken = uuid.NewString()
		if err := tx.db.Save(&t).Error; err != nil {
			return err
		}
		task = &t
		return nil
	})
	return task, err
}

func (r *KnowledgeAgentRepository) FinishTask(ctx context.Context, t *domain.KnowledgeTask, status, message string) error {
	res := r.db.WithContext(ctx).Model(&domain.KnowledgeTask{}).Where("id = ? AND status = 'running' AND lease_token = ?", t.ID, t.LeaseToken).
		Updates(map[string]any{"status": status, "error": message, "lease_until": nil, "available_at": time.Now().Add(time.Minute), "updated_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return domain.ErrKnowledgeConflict
	}
	return nil
}

func (r *KnowledgeAgentRepository) SaveTaskRange(ctx context.Context, t *domain.KnowledgeTask) error {
	return r.db.WithContext(ctx).Model(t).Where("status='running' AND lease_token = ?", t.LeaseToken).Select("MessageIDs").Updates(t).Error
}

func (r *KnowledgeAgentRepository) ListTasks(ctx context.Context, kbID, userID string) ([]domain.KnowledgeTask, error) {
	items := []domain.KnowledgeTask{}
	err := r.db.WithContext(ctx).Where("kb_id=? AND user_id=? AND kind IN ('extract','index')", kbID, userID).Order("created_at DESC").Limit(100).Find(&items).Error
	return items, err
}

func (r *KnowledgeAgentRepository) SetTaskStatus(ctx context.Context, kbID, id, userID, status string) error {
	q := r.db.WithContext(ctx).Model(&domain.KnowledgeTask{}).Where("kb_id=? AND id=? AND user_id=?", kbID, id, userID)
	if status == "pending" {
		q = q.Where("status IN ('failed','paused')")
	} else {
		q = q.Where("status IN ('pending','running','paused')")
	}
	values := map[string]any{"status": status, "lease_until": nil, "updated_at": time.Now()}
	if status == "pending" {
		values["attempts"] = 0
		values["available_at"] = time.Now()
		values["error"] = ""
	}
	res := q.Updates(values)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return domain.ErrKnowledgeConflict
	}
	return nil
}

func (r *KnowledgeAgentRepository) Change(ctx context.Context, kbID, id string, lock bool) (*domain.KnowledgeChange, error) {
	var c domain.KnowledgeChange
	q := r.db.WithContext(ctx).Where("kb_id=? AND id=?", kbID, id)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(&c).Error
	return &c, err
}

func (r *KnowledgeAgentRepository) Changes(ctx context.Context, kbID, status string) ([]domain.KnowledgeChange, error) {
	items := []domain.KnowledgeChange{}
	q := r.db.WithContext(ctx).Where("kb_id=?", kbID)
	if status != "" {
		q = q.Where("status=?", status)
	}
	err := q.Order("created_at DESC").Limit(200).Find(&items).Error
	return items, err
}

func (r *KnowledgeAgentRepository) SaveChange(ctx context.Context, c *domain.KnowledgeChange) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *KnowledgeAgentRepository) InsertChange(ctx context.Context, c *domain.KnowledgeChange) (bool, error) {
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(c)
	return result.RowsAffected == 1, result.Error
}

func (r *KnowledgeAgentRepository) ChangeByTask(ctx context.Context, kbID, taskID string) (*domain.KnowledgeChange, error) {
	var c domain.KnowledgeChange
	err := r.db.WithContext(ctx).Where("kb_id=? AND task_id=?", kbID, taskID).First(&c).Error
	return &c, err
}

func (r *KnowledgeAgentRepository) ReschedulePending(ctx context.Context, id string, due time.Time) error {
	return r.db.WithContext(ctx).Model(&domain.KnowledgeTask{}).Where("id=? AND status='pending' AND message_ids='[]'::jsonb", id).Update("available_at", due).Error
}

func (r *KnowledgeAgentRepository) AddSource(ctx context.Context, s *domain.KnowledgeSource) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *KnowledgeAgentRepository) Audit(ctx context.Context, kbID, userID, changeID, action, result string) error {
	return r.db.WithContext(ctx).Create(&domain.KnowledgeAudit{ID: uuid.NewString(), KBID: kbID, UserID: userID, ChangeID: changeID, Action: action, Result: result}).Error
}

func (r *KnowledgeAgentRepository) Audits(ctx context.Context, kbID string) ([]domain.KnowledgeAudit, error) {
	items := []domain.KnowledgeAudit{}
	err := r.db.WithContext(ctx).Where("kb_id=?", kbID).Order("created_at DESC").Limit(200).Find(&items).Error
	return items, err
}

func (r *KnowledgeAgentRepository) LockNode(ctx context.Context, kbID, id string) (*domain.Node, error) {
	var n domain.Node
	err := r.db.WithContext(ctx).Where("kb_id=? AND id=?", kbID, id).Clauses(clause.Locking{Strength: "UPDATE"}).First(&n).Error
	return &n, err
}

func (r *KnowledgeAgentRepository) ConsumeBudget(ctx context.Context, kbID, userID string, limit int) error {
	return r.Transaction(ctx, func(tx *KnowledgeAgentRepository) error {
		if _, err := tx.Policy(ctx, kbID, true); err != nil {
			return err
		}
		var count int64
		if err := tx.db.Model(&domain.KnowledgeAudit{}).Where("kb_id=? AND action='model_call' AND created_at >= date_trunc('day', now())", kbID).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(limit) {
			return domain.ErrKnowledgeBudget
		}
		return tx.Audit(ctx, kbID, userID, "", "model_call", "reserved")
	})
}

func (r *KnowledgeAgentRepository) AdvanceWatermark(ctx context.Context, t *domain.KnowledgeTask, last domain.ConversationMessage) error {
	res := r.db.WithContext(ctx).Model(&domain.KnowledgeConversationState{}).
		Where("conversation_id=? AND kb_id=? AND user_id=?", t.ConversationID, t.KBID, t.UserID).
		Where("watermark < ? OR (watermark = ? AND watermark_id <= ?)", last.CreatedAt, last.CreatedAt, last.ID).
		Updates(map[string]any{"watermark": last.CreatedAt, "watermark_id": last.ID, "updated_at": time.Now()})
	return res.Error
}

func (r *KnowledgeAgentRepository) ListStates(ctx context.Context) ([]domain.KnowledgeConversationState, error) {
	states := []domain.KnowledgeConversationState{}
	err := r.db.WithContext(ctx).Where("updated_at < now() - interval '5 minutes'").Order("updated_at").Limit(100).Find(&states).Error
	return states, err
}

func (r *KnowledgeAgentRepository) ReserveChat(ctx context.Context, kbID, userID, key string) (*domain.KnowledgeTask, error) {
	until := time.Now().Add(10 * time.Minute)
	t := &domain.KnowledgeTask{ID: uuid.NewString(), KBID: kbID, UserID: userID, Kind: "chat", Key: "chat:" + kbID + ":" + userID + ":" + key, Status: "running", LeaseToken: uuid.NewString(), LeaseUntil: &until, AvailableAt: time.Now(), MessageIDs: []string{}, ReleaseIDs: []string{}}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(t)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, domain.ErrKnowledgeDuplicate
	}
	return t, nil
}
