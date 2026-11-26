package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/chaitin/panda-wiki/config"
	"github.com/chaitin/panda-wiki/consts"
	"github.com/chaitin/panda-wiki/domain"
	"github.com/chaitin/panda-wiki/handler"
	"github.com/chaitin/panda-wiki/log"
	"github.com/chaitin/panda-wiki/middleware"
	"github.com/chaitin/panda-wiki/repo/pg"
	store "github.com/chaitin/panda-wiki/store/pg"
	"github.com/chaitin/panda-wiki/usecase"
)

// Exercises real JWT middleware, HTTP handlers, services, publication and PostgreSQL.
func TestKnowledgeHTTPEndToEnd(t *testing.T) {
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
	name := "pandawiki_agent_test_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	db, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn, _ := db.DB()
		_ = conn.Close()
		_ = admin.Exec("DROP DATABASE " + name + " WITH (FORCE)").Error
		conn, _ = admin.DB()
		_ = conn.Close()
	})
	dir := os.Getenv("PANDAWIKI_MIGRATIONS_DIR")
	if dir == "" {
		dir = "../../store/pg/migration"
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatal("missing migrations")
	}
	for _, file := range files {
		sql, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(string(sql)).Error; err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{Auth: config.AuthConfig{Type: "jwt", JWT: config.JWTConfig{Secret: uuid.NewString()}}}
	logger := log.NewLogger(cfg)
	wrapped := &store.DB{DB: db}
	repo := pg.NewKnowledgeAgentRepository(wrapped, logger)
	nodes := repo.Nodes()
	nav := pg.NewNavRepository(wrapped, logger)
	kbRepo := pg.NewKnowledgeBaseRepository(wrapped, cfg, logger, nil)
	kb, err := usecase.NewKnowledgeBaseUsecase(kbRepo, nodes, nav, nil, nil, nil, nil, logger, cfg)
	if err != nil {
		t.Fatal(err)
	}
	node := usecase.NewNodeUsecase(nodes, nav, nil, nil, nil, kbRepo, nil, nil, logger, nil, nil, nil, nil)
	agent := usecase.NewKnowledgeAgentUsecase(repo, node, kb, nil, nil, nav, logger)
	access := pg.NewUserAccessRepository(wrapped, logger)
	auth, err := middleware.NewAuthMiddleware(cfg, logger, access, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	base := handler.NewBaseHandler(e, logger, cfg, auth, nil, nil)
	NewKnowledgeAgentHandler(e, base, auth, agent, nil)
	for _, v := range []any{
		&domain.User{ID: "owner", Account: "owner", Role: consts.UserRoleAdmin},
		&domain.User{ID: "reader", Account: "reader", Role: consts.UserRoleUser},
		&domain.KnowledgeBase{ID: "kb", Name: "HTTP 测试库"}, &domain.Nav{ID: "nav", KbID: "kb", Name: "默认"},
		&domain.KnowledgePolicy{KBID: "kb", Mode: "manual", AllowModelTransfer: true, UserIDs: []string{"owner"}, FolderIDs: []string{"*"}, DailyCalls: 50},
		&domain.KnowledgeTask{ID: "task", KBID: "kb", UserID: "owner", Kind: "extract", Status: "done", Key: "fixture", AvailableAt: time.Now(), MessageIDs: []string{}, ReleaseIDs: []string{}},
		&domain.KnowledgeChange{ID: "change", KBID: "kb", UserID: "owner", TaskID: "task", Revision: 1, Status: "pending", ContentType: "md", After: "真实 HTTP 发布链路测试正文", Proposal: domain.KnowledgeExtraction{ShouldSave: true, Action: "CREATE", Title: "HTTP 测试知识", NavID: "nav", RiskLevel: "low"}},
	} {
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	token := func(id string) string {
		value, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"id": id, "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte(cfg.Auth.JWT.Secret))
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	request := func(method, path, user string, body any) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(payload))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		if user != "" {
			req.Header.Set(echo.HeaderAuthorization, "Bearer "+token(user))
		}
		response := httptest.NewRecorder()
		e.ServeHTTP(response, req)
		return response
	}
	if r := request(http.MethodGet, "/api/v1/knowledge-agent/changes?kb_id=kb", "", nil); r.Code != http.StatusUnauthorized {
		t.Fatal("anonymous access", r.Code)
	}
	if r := request(http.MethodGet, "/api/v1/knowledge-agent/changes?kb_id=kb", "reader", nil); r.Code != http.StatusForbidden {
		t.Fatal("reader access", r.Code)
	}
	if r := request(http.MethodGet, "/api/v1/knowledge-agent/changes?kb_id=kb", "owner", nil); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), "HTTP 测试知识") {
		t.Fatal("inbox unavailable", r.Code, r.Body.String())
	}
	review := domain.KnowledgeReviewReq{KBID: "kb", ID: "change", Revision: 1, IdempotencyKey: "http-approve"}
	for i := 0; i < 2; i++ {
		r := request(http.MethodPost, "/api/v1/knowledge-agent/review/approve", "owner", review)
		if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"status":"published"`) {
			t.Fatal("publication failed", r.Code, r.Body.String())
		}
	}
	var count int64
	if err := db.Model(&domain.NodeRelease{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("HTTP retry duplicated publication", count, err)
	}
	if r := request(http.MethodGet, "/api/v1/knowledge-agent/changes?kb_id=other", "owner", nil); r.Code != http.StatusForbidden {
		t.Fatal("invalid scope accepted", r.Code)
	}
}
