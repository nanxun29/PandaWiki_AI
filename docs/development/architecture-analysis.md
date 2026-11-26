# PandaWiki_AI 源码分析

分析日期：2026-11-26。基线：`f15493e8d84d998e0232bbc5bee9413ef37ba8ab`。
本报告区分源码结论与运行验证；运行结果见 progress.md。

## 整体架构与规范

`backend/` 使用 Go 1.24.3、Echo、Wire、GORM/PostgreSQL。HTTP 在 handler，业务在 usecase，持久化在 repo，基础客户端在 store。`web/` 是 pnpm 10.12.1 工作区：admin 为 React/Vite/Redux/React Router，app 为 Next.js；共享主题、图标和 UI 在 packages。管理界面使用 MUI 和 @ctzhian/ui，接口通过 Swagger 生成。根 AGENTS.md 要求中文沟通、小范围修改及提交前 make lint；它引用的 CLAUDE.md 在本提交不存在。

## 业务与调用位置

| 分析项 | 实际实现与复用边界 |
|---|---|
| 知识库管理 | handler/v1/knowledge_base.go → usecase/knowledge_base.go → repo/pg/knowledge_base.go；创建时通过 RAGService 创建 dataset，并配置前台访问规则。 |
| 目录和节点 | domain/node.go 的 Node 同时表示文件夹和文章，ParentID/Position 表示树，NavId 表示导航分组；repo/pg/nav.go 管理导航。必须校验目标目录、导航及节点属于同一库。 |
| 文档 CRUD | handler/v1/node.go → NodeUsecase → NodeRepository；创建必须保留 MaxNode 社区版数量限制。删除会处理发布记录、备份和索引清理，不向 Agent 开放。 |
| 草稿和发布 | nodes 保存工作内容；node_releases 保存发布快照；CreateKBRelease 调用 CreateNodeReleases，再通过 AsyncUpdateNodeReleaseVector 投递索引任务。工作内容更新与正式发布相互独立。 |
| 历史与回滚 | node_releases/node_release_backups 已存在，但 admin 历史页面调用 /api/pro/v1/node/release 接口；不能把数据表存在等同于社区版提供完整历史 API。备份 cron 默认清理 30 天前记录。AI 审计快照独立保存。 |
| 正文和编辑器 | domain.Node.Content 为字符串，Meta.ContentType 区分 md 与 HTML；admin 的 document/editor/edit/Wrap.tsx 复用 EditorMarkdown 及富文本编辑器，getMarkdown/getHTML 支持转换。禁止对富文本进行整篇 Markdown 往返转换。 |
| AI 聊天 UI | app/src/components/QaModal/AiQaContent.tsx 使用 SSEClient、markdown2 渲染、来源引用、会话恢复、反馈与中断；h5Chat/widget 有各自适配。不能新建第二套会话存储或聊天引擎。 |
| 聊天调用 | POST /share/v1/chat/message → ShareChatHandler.ChatMessage → ChatUsecase.Chat → BuildConversationMessageWithRAG → ChatWithAgent。后者实际为 BaseChatModel.Stream 的消费循环，并无知识工具调度。 |
| 会话存储 | domain/conversation.go 与 repo/pg/conversation.go 的 conversations、conversation_messages、conversation_references；现有前台会话通过 nonce 校验，不能用 nonce 代替后台用户权限。 |
| AI 搜索与问答 | ChatUsecase.Search/ChatRagOnly、LLMUsecase.GetRankNodes，复用 RAGService.QueryRecords；按 group_ids 过滤检索权限，再映射发布节点和引用。 |
| AI 创作 | usecase/creation.go 的 TextCreation/TabComplete、NodeUsecase.SummaryNode/StreamSummaryNode；继续复用模型与编辑器。 |
| 模型 | usecase/model.go 和 domain/model.go；通过 ModelKit 获取 Eino BaseChatModel，已有自动/手动配置、模型供应商与用量统计。新增任务仅引用已配置模型，不另存密钥。 |
| RAG 检索 | store/rag/ct.go 通过 raglite-go-sdk 向 CT RAG HTTP 服务发送 dataset/query/history/group_ids，TopK=10；LLMUsecase 将切片映射到 node_releases。 |
| Embedding/Rerank/切片 | Wiki 配置并同步模型到 RAG 服务；切片、向量计算与重排不在 Wiki 主仓库实现。不能声称本仓库包含完整向量引擎或可独立离线运行。 |
| 发布与索引 | repo/mq/rag.go → NATS VectorTaskTopic → handler/mq/rag.go → RAGService.UpsertRecords；HTML 在此转换 Markdown。消费端检查最新发布、请求及内容幂等，写回 doc_id 并清理旧 doc。 |
| 索引状态 | handler/mq/rag_doc_update.go 接收 raglite.events.doc.update；NodeUsecase.SyncRagNodeStatus 与定时任务补偿状态。草稿不应发向量任务。 |
| 后台认证 | middleware/jwt.go 支持 JWT/API token；ValidateKBUserPerm 和 repo/pg/user_access.go 校验后台用户与 kb_users，DocManage 可编辑发布，FullControl 可设置授权。 |
| 前台权限 | middleware/share_auth.go、usecase/node.go 的 ValidateNodePerm/GetNodeIdsByAuthId；auths/auth_groups 与后台 users 是不同身份体系，访客不可自动映射为管理员。 |
| 异步和定时 | mq/nats 包为 JetStream 提供 durable、ACK、超时和延迟重投；handler/mq/cron.go 使用 robfig/cron。复用这些设施，持久任务用于可见状态与重启恢复。 |
| Tool Calling/MCP | 依赖中有 Eino 与 mcp-go；repo/pg/mcp.go 只统计 mcp_calls。MCP 设置和 pro 引用不证明社区版具备可复用的知识写入工具。 |
| 商业边界 | .gitmodules 指向私有 backend/pro；domain/license.go 保留 MaxKb/MaxNode 等限制，middleware/jwt.go 有 edition 校验。不得下载私有商业代码或移除限制。 |
| 构建部署 | Dockerfile.api 构建 API 和 migrate，启动前迁移；Dockerfile.consumer 构建消费服务。仓库缺少 Makefile 引用的 backend/deploy 目录；README 安装器为外部资源。CI 分别构建社区版 api/consumer 与前端。 |
| 前端约定 | admin/src/router.tsx 和 components/Sidebar/index.tsx 管理路由与菜单，store/slices/config 提供当前 kb_id/权限；不手改生成客户端，流接口使用包装。 |

## 复用、扩展与新增

- 直接复用：知识库/节点/导航、现有编辑器、发布快照、模型系统、RAG 适配器、NATS、cron、认证、主题及响应格式。
- 扩展：后台会话所有权、原 ChatUsecase 的授权 Agent 分支、发布事务和索引待投递记录、所有节点写路径的修订标记。
- 新增：受控知识工具、结构化提取、策略、消息水位、持久任务、收件箱/审核、AI 快照与来源审计。

## 风险与验证门槛

现有发布流程的数据库提交与 MQ 投递不是一个事务；新功能必须记录可恢复的索引投递意图。行锁本身不能防止过期建议覆盖新内容，需在同一事务校验修订。正文可同时为 HTML/Markdown，局部修改须保留原格式和附件。RAG 是独立外部服务，缺失时不得把检索错误解释成没有相似文档。默认日志包含部分模型上下文，新功能不得新增原始敏感内容日志。后台私有会话不得经前台 nonce 接口暴露。模型供应商、本地模型、真实 RAG、Docker 启动均需实测，不能由静态分析推导通过。
