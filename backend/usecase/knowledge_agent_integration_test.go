package usecase

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/chaitin/panda-wiki/config"
	"github.com/chaitin/panda-wiki/consts"
	"github.com/chaitin/panda-wiki/domain"
	"github.com/chaitin/panda-wiki/log"
	mqClient "github.com/chaitin/panda-wiki/mq/nats"
	"github.com/chaitin/panda-wiki/mq/types"
	mqRepo "github.com/chaitin/panda-wiki/repo/mq"
	"github.com/chaitin/panda-wiki/repo/pg"
	store "github.com/chaitin/panda-wiki/store/pg"
)

// Uses only an explicitly supplied test server and a new uniquely named database.
func agentIntegration(t *testing.T) (*KnowledgeAgentUsecase, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("PANDAWIKI_TEST_DSN")
	if dsn == "" {
		t.Skip("PANDAWIKI_TEST_DSN is not configured")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "postgresql" {
		t.Fatal("test DSN must be a postgresql URL")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	name := "pandawiki_agent_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	db, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if conn, e := db.DB(); e == nil {
			_ = conn.Close()
		}
		_ = admin.Exec("DROP DATABASE " + name + " WITH (FORCE)").Error
		if conn, e := admin.DB(); e == nil {
			_ = conn.Close()
		}
	})
	dir := os.Getenv("PANDAWIKI_MIGRATIONS_DIR")
	if dir == "" {
		dir = "../store/pg/migration"
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatal("migration files missing", err)
	}
	for _, file := range files {
		sql, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(string(sql)).Error; err != nil {
			t.Fatalf("migration %s: %v", file, err)
		}
	}
	logger := log.NewLogger(&config.Config{})
	wrapped := &store.DB{DB: db}
	kbRepo := pg.NewKnowledgeBaseRepository(wrapped, &config.Config{}, logger, nil)
	repo := pg.NewKnowledgeAgentRepository(wrapped, logger)
	u := &KnowledgeAgentUsecase{repo: repo, node: &NodeUsecase{nodeRepo: repo.Nodes()}, kb: &KnowledgeBaseUsecase{repo: kbRepo, nodeRepo: repo.Nodes()}, nav: pg.NewNavRepository(wrapped, logger), logger: logger}
	for _, value := range []any{
		&domain.User{ID: "owner", Account: "owner", Role: consts.UserRoleAdmin},
		&domain.User{ID: "reviewer", Account: "reviewer", Role: consts.UserRoleAdmin},
		&domain.User{ID: "outsider", Account: "outsider", Role: consts.UserRoleUser},
		&domain.KnowledgeBase{ID: "kb", Name: "测试知识库"},
		&domain.Nav{ID: "nav", KbID: "kb", Name: "默认导航"},
		&domain.Conversation{ID: "conversation", KBID: "kb", AdminUserID: "owner", Nonce: "private-nonce"},
		&domain.KnowledgeConversationState{ConversationID: "conversation", KBID: "kb", UserID: "owner", Watermark: time.Unix(0, 0)},
		&domain.ConversationMessage{ID: "user-message", KBID: "kb", ConversationID: "conversation", Role: schema.User, Content: "已确认故障修复", CreatedAt: time.Now().Add(-time.Second)},
		&domain.ConversationMessage{ID: "assistant-message", KBID: "kb", ConversationID: "conversation", Role: schema.Assistant, Content: "已收到确认", CreatedAt: time.Now()},
		&domain.KnowledgePolicy{KBID: "kb", Mode: "draft", AllowModelTransfer: true, UserIDs: []string{"owner"}, FolderIDs: []string{"*"}, DailyCalls: 50},
	} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	return u, db
}

func proposedChange(t *testing.T, u *KnowledgeAgentUsecase, target string) *domain.KnowledgeChange {
	t.Helper()
	ctx := context.Background()
	task := &domain.KnowledgeTask{ID: uuid.NewString(), KBID: "kb", UserID: "owner", ConversationID: "conversation", Kind: "extract", Key: uuid.NewString(), Status: "done", AvailableAt: time.Now(), MessageIDs: []string{"user-message", "assistant-message"}, ReleaseIDs: []string{}}
	if err := u.repo.Enqueue(ctx, task); err != nil {
		t.Fatal(err)
	}
	e := domain.KnowledgeExtraction{ShouldSave: true, Title: "故障修复", Content: "已确认配置修复并通过验证。", Action: "CREATE", NavID: "nav", RiskLevel: "low", VerificationStatus: "user_confirmed", Reason: "用户确认修复", SourceMessageIDs: []string{"user-message"}}
	if target != "" {
		e.Action = "APPEND"
		e.TargetDocumentID = target
	}
	c, err := u.buildChange(ctx, task, e, "configured-model")
	if err != nil {
		t.Fatal(err)
	}
	if err := u.repo.SaveChange(ctx, c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestKnowledgeIntegrationPublishAtomicAndIdempotent(t *testing.T) {
	u, db := agentIntegration(t)
	c := proposedChange(t, u, "")
	ctx := context.Background()
	var count int64
	if err := db.Model(&domain.Node{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("unreviewed proposal materialized a document", err)
	}
	req := domain.KnowledgeReviewReq{KBID: "kb", ID: c.ID, Revision: c.Revision, IdempotencyKey: "approve-once"}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := u.Review(ctx, req, "reviewer", "approve"); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"nodes", "node_releases", "kb_releases"} {
		if err := db.Table(table).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	if err := db.Model(&domain.KnowledgeTask{}).Where("kind='index' AND status='pending'").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("missing or duplicate atomic outbox", count, err)
	}
	if _, err := u.Review(ctx, req, "outsider", "approve"); !errors.Is(err, domain.ErrKnowledgeForbidden) {
		t.Fatal("outsider could publish", err)
	}
}

func TestKnowledgeIntegrationVersionAndPolicyConflicts(t *testing.T) {
	u, db := agentIntegration(t)
	ctx := context.Background()
	format := "md"
	id, err := u.node.Create(ctx, &domain.CreateNodeReq{KBID: "kb", NavId: "nav", Type: domain.NodeTypeDocument, Name: "已有文档", Content: "保留图片 ![拓扑](network.png)", ContentType: &format, MaxNode: 300}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	c := proposedChange(t, u, id)
	manual := "用户并发修改的正文"
	if err := u.node.Update(ctx, &domain.UpdateNodeReq{ID: id, KBID: "kb", Content: &manual}, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := u.Review(ctx, domain.KnowledgeReviewReq{KBID: "kb", ID: c.ID, Revision: 1, IdempotencyKey: "stale"}, "reviewer", "approve"); !errors.Is(err, domain.ErrKnowledgeConflict) {
		t.Fatal("stale edit was not rejected", err)
	}
	n, err := u.repo.Nodes().GetNodeByID(ctx, id)
	if err != nil || n.Content != manual {
		t.Fatal("manual edit overwritten", err)
	}
	c = proposedChange(t, u, "")
	if err := db.Model(&domain.KnowledgePolicy{}).Where("kb_id='kb'").Update("mode", "off").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := u.Review(ctx, domain.KnowledgeReviewReq{KBID: "kb", ID: c.ID, Revision: 1, IdempotencyKey: "revoked"}, "reviewer", "approve"); !errors.Is(err, domain.ErrKnowledgeForbidden) {
		t.Fatal("revoked policy allowed publish", err)
	}
}

func TestKnowledgeIntegrationTransactionFailureRollsBack(t *testing.T) {
	u, db := agentIntegration(t)
	c := proposedChange(t, u, "")
	if err := db.Exec(`CREATE FUNCTION fail_agent_release() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected publication failure'; END $$; CREATE TRIGGER fail_agent_release BEFORE INSERT ON kb_releases FOR EACH ROW EXECUTE FUNCTION fail_agent_release()`).Error; err != nil {
		t.Fatal(err)
	}
	_, err := u.Review(context.Background(), domain.KnowledgeReviewReq{KBID: "kb", ID: c.ID, Revision: 1, IdempotencyKey: "fail"}, "reviewer", "approve")
	if err == nil {
		t.Fatal("expected injected failure")
	}
	for _, table := range []string{"nodes", "node_releases", "kb_releases"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("partial commit in %s", table)
		}
	}
	stored, err := u.repo.Change(context.Background(), "kb", c.ID, false)
	if err != nil || stored.Status != "pending" {
		t.Fatal("proposal status committed despite failed publication", err)
	}
}

func TestKnowledgeIntegrationPrivateConversationAndSources(t *testing.T) {
	u, db := agentIntegration(t)
	ctx := context.Background()
	if _, err := u.Messages(ctx, "kb", "conversation", "reviewer"); !errors.Is(err, domain.ErrKnowledgeForbidden) {
		t.Fatal("reviewer read private transcript", err)
	}
	r := pg.NewConversationRepository(&store.DB{DB: db}, u.logger)
	if err := r.ValidateConversationNonce(ctx, "conversation", "private-nonce"); !errors.Is(err, domain.ErrKnowledgeForbidden) {
		t.Fatal("public nonce exposed private conversation", err)
	}
	if _, err := r.GetConversationDetail(ctx, "kb", "conversation"); !errors.Is(err, domain.ErrKnowledgeForbidden) {
		t.Fatal("public read exposed private conversation", err)
	}
}

func TestKnowledgeIntegrationTaskLeaseCancelAndRecovery(t *testing.T) {
	u, db := agentIntegration(t)
	ctx := context.Background()
	if _, err := u.repo.ReserveChat(ctx, "kb", "owner", "same-request"); err != nil {
		t.Fatal(err)
	}
	if _, err := u.repo.ReserveChat(ctx, "kb", "owner", "same-request"); !errors.Is(err, domain.ErrKnowledgeDuplicate) {
		t.Fatal("duplicate chat request accepted", err)
	}
	task, err := u.QueueExtraction(ctx, "kb", "conversation", "owner", "one", true)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := u.QueueExtraction(ctx, "kb", "conversation", "owner", "two", true)
	if err != nil || duplicate.ID != task.ID {
		t.Fatal("task coalescing failed", err)
	}
	first, err := u.repo.Claim(ctx, task.ID)
	if err != nil || first == nil {
		t.Fatal("could not claim", err)
	}
	if err := db.Model(&domain.KnowledgeTask{}).Where("id=?", task.ID).Update("lease_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	recovered, err := u.repo.Claim(ctx, task.ID)
	if err != nil || recovered == nil || recovered.LeaseToken == first.LeaseToken {
		t.Fatal("lease not fenced", err)
	}
	if err := u.repo.FinishTask(ctx, first, "done", ""); !errors.Is(err, domain.ErrKnowledgeConflict) {
		t.Fatal("expired worker committed", err)
	}
	if err := u.TaskAction(ctx, "kb", task.ID, "owner", "cancel"); err != nil {
		t.Fatal(err)
	}
	if err := u.repo.FinishTask(ctx, recovered, "done", ""); !errors.Is(err, domain.ErrKnowledgeConflict) {
		t.Fatal("cancelled worker committed", err)
	}
}

func TestKnowledgeIntegrationRollbackAndRegenerate(t *testing.T) {
	u, _ := agentIntegration(t)
	ctx := context.Background()
	format := "md"
	id, err := u.node.Create(ctx, &domain.CreateNodeReq{KBID: "kb", NavId: "nav", Type: domain.NodeTypeDocument, Name: "部署流程", Content: "保留原始流程", ContentType: &format, MaxNode: 300}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	c := proposedChange(t, u, id)
	published, err := u.Review(ctx, domain.KnowledgeReviewReq{KBID: "kb", ID: c.ID, Revision: 1, IdempotencyKey: "publish"}, "owner", "approve")
	if err != nil {
		t.Fatal(err)
	}
	if err := u.repo.Nodes().Update(ctx, id, map[string]any{"rag_info": "{}"}); err != nil {
		t.Fatal(err)
	}
	reverse, err := u.Rollback(ctx, domain.KnowledgeReviewReq{KBID: "kb", ID: c.ID, IdempotencyKey: "rollback"}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if reverse.After != c.Before || reverse.Before != published.After {
		t.Fatal("wrong rollback snapshot")
	}
	if _, err := u.Review(ctx, domain.KnowledgeReviewReq{KBID: "kb", ID: reverse.ID, Revision: 1, IdempotencyKey: "apply-rollback"}, "owner", "approve"); err != nil {
		t.Fatal(err)
	}
	n, err := u.repo.Nodes().GetNodeByID(ctx, id)
	if err != nil || n.Content != c.Before {
		t.Fatal("rollback did not restore content", err)
	}
	draft := proposedChange(t, u, "")
	job, err := u.Regenerate(ctx, domain.KnowledgeReviewReq{KBID: "kb", ID: draft.ID, Revision: 1, IdempotencyKey: "regenerate"}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(job.MessageIDs) != 2 || job.Status != "pending" {
		t.Fatal("regeneration lost source range")
	}
}

func TestKnowledgeIntegrationDuplicateProposalsBudgetAndToolBoundary(t *testing.T) {
	u, _ := agentIntegration(t)
	ctx := context.Background()
	first := proposedChange(t, u, "")
	second := *first
	second.ID = uuid.NewString()
	job := &domain.KnowledgeTask{ID: uuid.NewString(), KBID: "kb", UserID: "owner", ConversationID: "conversation", Kind: "extract", Key: uuid.NewString(), Status: "done", AvailableAt: time.Now(), MessageIDs: []string{}, ReleaseIDs: []string{}}
	if err := u.repo.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}
	second.TaskID = job.ID
	inserted, err := u.repo.InsertChange(ctx, &second)
	if err != nil || inserted {
		t.Fatal("duplicate pending knowledge accepted", err)
	}
	if err := u.repo.ConsumeBudget(ctx, "kb", "owner", 1); err != nil {
		t.Fatal(err)
	}
	if err := u.repo.ConsumeBudget(ctx, "kb", "owner", 1); !errors.Is(err, domain.ErrKnowledgeBudget) {
		t.Fatal("budget exceeded", err)
	}
	req := &domain.ChatRequest{KBID: "kb", AdminUserID: "owner", ConversationID: "conversation"}
	if _, err := u.executeTool(ctx, req, knowledgeToolCall{Tool: "execute_shell", Query: "rm -rf /"}, 0); !errors.Is(err, domain.ErrKnowledgeInvalid) {
		t.Fatal("unregistered tool accepted", err)
	}
	t.Setenv("READONLY", "true")
	if _, err := u.authorize(ctx, u.repo, "kb", "owner"); !errors.Is(err, domain.ErrKnowledgeForbidden) {
		t.Fatal("worker bypassed read-only mode", err)
	}
}

func TestKnowledgeIntegrationOriginalNATSOutbox(t *testing.T) {
	server := os.Getenv("PANDAWIKI_TEST_NATS")
	if server == "" {
		t.Skip("PANDAWIKI_TEST_NATS is not configured")
	}
	u, _ := agentIntegration(t)
	ctx := context.Background()
	cfg := &config.Config{MQ: config.MQConfig{Type: "nats", NATS: config.NATSConfig{Server: server}}}
	producer, err := mqClient.NewMQProducer(cfg, u.logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = producer.Close() })
	consumer, err := mqClient.NewMQConsumer(u.logger, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = consumer.Close() })
	var deliveries atomic.Int32
	received := make(chan struct{}, 1)
	if err := consumer.RegisterHandler(domain.VectorTaskTopic, func(ctx context.Context, msg types.Message) error {
		deliveries.Add(1)
		select {
		case received <- struct{}{}:
		default:
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	u.kb.ragRepo = mqRepo.NewRAGRepository(producer)
	job := &domain.KnowledgeTask{ID: uuid.NewString(), KBID: "kb", UserID: "owner", Kind: "index", Key: uuid.NewString(), Status: "pending", AvailableAt: time.Now(), MessageIDs: []string{}, ReleaseIDs: []string{"release-to-index"}}
	if err := u.repo.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := u.ProcessTask(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("outbox did not reach the original vector queue")
	}
	if err := u.ProcessTask(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if deliveries.Load() != 1 {
		t.Fatal("duplicate outbox delivery")
	}
	stored, err := u.repo.Task(ctx, "kb", job.ID, false)
	if err != nil || stored.Status != "done" {
		t.Fatal("outbox completion not persisted", err)
	}
}
