# API 设计

统一前缀 /api/v1/knowledge-agent。沿用 PWResponse，SSE 沿用原 type/content/chunk_result。
仅接受后台 JWT；每个请求以服务端认证用户重新检查实际知识库权限。

| 方法与路径 | 行为 |
|---|---|
| GET /policy?kb_id= | 获取策略和当前后台用户 ID。 |
| PUT /policy | FullControl 用户更新 mode/model_id/allow_model_transfer/user_ids/folder_ids/daily_calls。 |
| POST /chat | kb_id、conversation_id（首轮可空）、message、idempotency_key；原聊天服务 SSE。 |
| GET /messages | kb_id/conversation_id，仅会话所有者。 |
| POST /extract | kb_id/conversation_id/idempotency_key，提交整理任务或返回没有新增完整轮次。 |
| GET /changes | kb_id/status，返回收件箱候选。 |
| POST /review/{edit,approve,reject} | kb_id/id/revision/idempotency_key；可附 after，edit 支持 title/nav_id/parent_id/target_document_id。切换目标后先保存再编辑。 |
| POST /changes/regenerate | 原建议 ID/revision/idempotency_key，来源所有者重新提取冻结范围。 |
| POST /changes/rollback | 已发布建议 ID/idempotency_key，生成待审核反向变更，不删除新建文档。 |
| GET /tasks | 当前用户的提取与索引投递状态。 |
| POST /tasks/{retry,cancel} | kb_id/id，重试失败或暂停任务，取消未完成任务。 |
| GET /audits | 当前知识库的操作元数据。 |

400 参数/结构化输出/敏感内容错误；403 未授权；404 来源或资源不存在；409 文档或建议版本冲突；429 预算耗尽；500 返回通用操作失败信息。

新增 SSE 事件 knowledge_tool 表示工具状态，原 data/chunk_result/done/error 保持兼容。工具状态不是文档发布成功。只有审核事务提交后才返回 published，索引任务 done 仅代表投递完成，最终学习状态沿用原文档接口。

后端 Swagger 为生成物。管理端 knowledgeAgent.ts 封装认证 HTTP 与既有 SSEClient，不手工修改原生成客户端。原 pnpm api 同时访问商业版 Swagger，社区版可通过本项目的离线生成配置仅生成新增接口。
