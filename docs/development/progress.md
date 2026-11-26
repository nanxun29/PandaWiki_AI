# PandaWiki_AI 开发进度

记录日期：2026-11-26。源码基线 f15493e8，工作分支 feature/ai-knowledge-agent。

## 当前交付状态

两个 P0 的功能代码、迁移、管理界面、API、测试与中文文档已写入。核心业务与隔离数据库测试通过；真实模型/RAG、完整部署和浏览器验收尚未完成，不能把整个产品验收标为完成。

## 已完成与复用

- 阶段一：克隆目标仓库，初始工作区干净；创建开发分支，保留 origin 并添加 upstream。已完成24项源码分析和原始后端构建；原项目完整启动及 AI/RAG 运行验证仍受外部服务限制。
- 阶段二：复用 ChatUsecase、会话存储、ModelKit、原 RAG、编辑器及文档发布服务；新增后台授权工具、收件箱、Diff、编辑/批准/拒绝、事务发布 outbox、并发修订与来源审计。
- 阶段三：实现增量水位、持久任务、NATS/cron 恢复、结构化提取、目录推荐、相似检索、待审核内容去重、策略/预算、取消/重试、重新提取及反向回滚建议。
- 前台原问答布局保持，聊天气泡抽为共享组件；管理端新增 /knowledge-agent，沿用现有认证、SSE、Markdown、主题、路由和编辑器。所有写入都经人工审核，新功能默认关闭。
- 保留 AGPL-3.0、原作者声明和商业许可检查；README 标明非官方独立衍生项目并记录修改日期。

## 主要修改文件

| 子系统 | 文件 |
|---|---|
| 新业务 | backend/usecase/knowledge_agent.go、knowledge_tools.go、knowledge_extractor.go、knowledge_history.go |
| 数据与规则 | backend/domain/knowledge_agent.go、backend/repo/pg/knowledge_agent.go、backend/pkg/knowledge/ |
| 接口/任务 | backend/handler/v1/knowledge_agent.go、backend/handler/mq/knowledge_agent.go；原 provider、Wire 和 NATS 注册 |
| 兼容接缝 | 原 chat.go、knowledge_base.go、会话仓库、节点修订字段、RAG 环境凭据配置 |
| 迁移 | backend/store/pg/migration/000039_knowledge_agent.{up,down}.sql |
| 管理页面 | web/admin/src/pages/knowledge-agent/、src/request/knowledgeAgent.ts、共享 conversation 气泡 |
| 交付 | 六份 development 文档、部署/使用说明、deploy 模板、README、CHANGELOG、验证脚本及社区版 CI |

## 实际执行的验证

| 检查 | 实际结果 |
|---|---|
| 原始提交 Go build ./... | 在原始源码归档上成功；工具链 Go1.24.3，临时归档禁用 VCS stamp。 |
| 新版本 Go build ./... | 已成功，包含 API、consumer 和 migrate。 |
| 受影响 Go 测试 | config/domain/pkg/knowledge/usecase/handler/mq/handler/v1 等通过；无测试服务环境变量时集成用例显式 Skip。 |
| 完整 go test ./... -timeout90s | 失败：上游 pkg/bot/discord.TestDiscord 第29行 select{} 永久等待。没有修改或伪称此测试通过。 |
| 全量测试排除该人工 Discord 用例 | go test ./... -skip '^TestDiscord$' -timeout90s 成功。 |
| PostgreSQL/NATS 真实集成 | 8组通过：迁移、未审核不生成正式文档、并发批准幂等、版本/权限冲突、事务失败回滚、私有来源、租约恢复/取消、回滚/重新提取、去重/预算/工具边界及原向量队列 outbox。 |
| HTTP 端到端 | 使用真实 JWT 中间件、Echo、业务服务及 PostgreSQL，通过匿名/只读拒绝、收件箱读取、审核发布、重复批准和非法库范围场景。 |
| golangci-lint | 基于基线的增量检查为 0 issues；使用 v2.1.0。 |
| make lint | 已执行；社区 Swagger 和 Wire 生成成功，随后 generate_pro 因缺失商业子模块 handler 目录失败。未下载商业代码或绕过授权。 |
| 管理端 TypeScript/ESLint | 通过；新页面与请求包装无 ESLint 问题。 |
| 管理端构建 | pnpm build --minify=false 与最终默认 pnpm build 均成功；默认压缩构建约6分18秒，使用3GB Node堆及16MB半空间配置。 |
| 前台改动文件 ESLint | 0 errors，保留原文件的1条未使用 theme 警告。 |
| pnpm api | 成功从本地 Swagger 生成新增社区 API 契约，避免依赖商业 Swagger 服务。 |
| Compose 配置 | 使用明确标记的测试占位环境执行 config --quiet 成功，未以占位配置启动业务服务。 |
| UI 规则检测/git diff --check | 已通过；浏览器桌面/移动端完整流程尚未执行。 |

## 发现并处理的问题

1. 当前工作区根目录没有源码且 .git 受保护，因此项目放在 PandaWiki_AI 子目录，未覆盖根目录或其他项目。
2. 沙箱普通网络命令不能下载依赖；使用获准的下载/隔离 Docker 环境取得工具和依赖，缓存保存在工作区外层 .devtools，不进入项目提交。
3. /tmp 为2GB内存文件系统，Go 缓存耗尽空间后改放磁盘；默认 Node 堆内存曾不足，已使用受限内存与无压缩构建验证，不将失败隐藏为成功。
4. 一次扩展集成测试因 docker cp 把新迁移复制到嵌套目录而读取旧版本，已修正为复制目录内容并重跑通过。
5. 静态检查发现两处新代码问题，已修复并重新检查。
6. 节点修订覆盖业务变更，单纯 RAG 学习状态不会使合法回滚失效；防止学习回调导致全部建议产生无意义冲突。

## 未完成验收与下一步

- 没有可用的实际模型配置或真实 CT RAG 服务凭据，尚未验证真实模型提取质量、Ollama、发布后最终学习与问答召回。没有引入假 RAG 或伪造模型结果冒充这些验收。
- 尚未执行完整 Compose 启动、镜像发布、浏览器真实账号流程及移动端视觉验收。部署模板依赖既有兼容 RAG/MinIO 镜像和配置，不能声称已完成生产部署。
- first-version CREATE 不支持 AI 自动删除；复杂 HTML 更新、冲突和合并保留人工审核。智能自动发布、自主维护、MCP 和外部 Agent 属于后续阶段。
- 本轮仅在本地开发分支按模块整理提交，未创建远程 PR、推送、自动合并或发布。

获得真实服务配置后，应按任务书15个验收场景补齐端到端证据，再决定是否进入自动发布与自主维护。
