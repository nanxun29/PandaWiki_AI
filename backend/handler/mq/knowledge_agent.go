package mq

import (
	"context"
	"encoding/json"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/chaitin/panda-wiki/domain"
	"github.com/chaitin/panda-wiki/log"
	"github.com/chaitin/panda-wiki/mq"
	"github.com/chaitin/panda-wiki/mq/types"
	"github.com/chaitin/panda-wiki/usecase"
)

type KnowledgeAgentHandler struct {
	agent    *usecase.KnowledgeAgentUsecase
	producer mq.MQProducer
	logger   *log.Logger
}

func NewKnowledgeAgentHandler(agent *usecase.KnowledgeAgentUsecase, consumer mq.MQConsumer, producer mq.MQProducer, logger *log.Logger) (*KnowledgeAgentHandler, error) {
	h := &KnowledgeAgentHandler{agent: agent, producer: producer, logger: logger}
	if err := consumer.RegisterHandler(domain.KnowledgeTaskTopic, h.Handle); err != nil {
		return nil, err
	}
	c := cron.New()
	if _, err := c.AddFunc("* * * * *", h.Dispatch); err != nil {
		return nil, err
	}
	c.Start()
	go h.Dispatch()
	return h, nil
}

func (h *KnowledgeAgentHandler) Handle(ctx context.Context, msg types.Message) error {
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.GetData(), &payload); err != nil {
		return nil
	}
	return h.agent.ProcessTask(ctx, payload.ID)
}

// Dispatch recovers committed-but-unpublished intents and expired task leases.
func (h *KnowledgeAgentHandler) Dispatch() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := h.agent.ScanConversations(ctx); err != nil {
		h.logger.Error("scan knowledge conversations failed")
	}
	tasks, err := h.agent.DueTasks(ctx)
	if err != nil {
		h.logger.Error("load knowledge tasks failed")
		return
	}
	for _, task := range tasks {
		payload, _ := json.Marshal(struct {
			ID string `json:"id"`
		}{task.ID})
		// A fresh dispatch slot avoids a lost lease being hidden by JetStream's dedupe window.
		key := task.ID + ":" + time.Now().UTC().Format("200601021504")
		if err := h.producer.Produce(ctx, domain.KnowledgeTaskTopic, key, payload); err != nil {
			h.logger.Error("dispatch knowledge task failed")
			return
		}
	}
}
