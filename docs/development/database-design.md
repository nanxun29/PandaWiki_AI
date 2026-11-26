# 数据库设计

迁移：backend/store/pg/migration/000039_knowledge_agent.up.sql。

| 对象 | 作用 |
|---|---|
| nodes.revision | 单调修订号；BEFORE UPDATE 触发器覆盖正文、标题、业务元数据、目录、权限和发布状态等变更。单纯 RAG 学习状态变化不使编辑建议或回滚失效。 |
| conversations.admin_user_id | 后台助手会话所有者，空字符串保持原有会话行为。 |
| knowledge_agent_policies | 每库模式、模型引用、数据传输许可、授权用户/目录及每日调用预算。 |
| knowledge_agent_conversation_states | 会话所有权和 (watermark,watermark_id) 增量位置。 |
| knowledge_agent_tasks | 提取任务、聊天幂等凭据和索引投递 outbox；状态、重试、租约 token、冻结消息和发布快照 ID。 |
| knowledge_agent_changes | 收件箱与审核候选，包含结构化建议、前后正文、格式、目标基线、已发布修订号和审核人。 |
| knowledge_agent_sources | 候选与真实会话/消息的权限受控关联；不存聊天全文。 |
| knowledge_agent_audits | 操作人、动作、结果、模型调用预算预留记录；不存密钥或原始提示词。 |

每个会话最多一个 pending/running/paused 提取任务；任务 task_key 唯一；每任务最多一个建议。待审核候选使用内容指纹去重，来源消息对每个候选唯一。任务状态为 pending/running/done/failed/cancelled/paused；建议为 pending/published/rejected。

批准后新建正式节点与发布快照，沿用 node_releases、kb_releases、kb_release_node_releases、nav_releases。已有文档候选不会预先覆盖 nodes 工作内容，未审核内容没有向量索引。

先备份 PostgreSQL 和附件。迁移不删既有数据。down 迁移会删除 AI 候选、任务和审计，仅供运维明确选择并导出数据后使用，应用不会自动回退迁移。部署回退优先保留新增表，关闭策略并回退应用镜像。
