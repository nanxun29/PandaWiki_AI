# PandaWiki_AI 中文部署说明

修改日期：2026-11-26。本项目是 PandaWiki 社区版的非官方衍生项目，沿用 AGPL-3.0。

## 升级已有部署

1. 备份 PostgreSQL、MinIO 附件和原部署配置，保存当前镜像版本。
2. 使用 Go 1.24.3、Node 22 或兼容版本、pnpm 10.12.1 构建社区版。不要初始化 backend/pro 私有子模块。
3. 在 web 执行 pnpm install --frozen-lockfile、pnpm build；在 backend 执行 go test ./... 和 go build ./...。低内存机器可用 NODE_OPTIONS=--max-old-space-size=3000 配合 admin 的 pnpm build --minify=false 验证；正式发布应在资源充足的构建机运行默认压缩构建。
4. 以原有 backend/Dockerfile.api、Dockerfile.consumer、web/admin/Dockerfile、web/app/Dockerfile 构建替换镜像。保留原数据卷和 RAG、Redis、NATS、MinIO、Caddy 拓扑。
5. 先启动 API，原迁移入口自动执行新增 000039 迁移，再启动 consumer 和前端。不要让旧 consumer 与升级后的消费者混用。
6. 初始策略为关闭。在后台配置模型后，进入知识收件箱的管理设置，显式选择模式、用户、目录和数据传输许可。

新增提取消费者使用独立 JetStream knowledge stream，发布任务通过原 vector stream 更新索引。NATS 账号需要创建 stream/consumer 的现有权限。后端、消费服务都必须部署同一代码版本。新聊天 SSE 路径已加入管理端 Nginx 流式代理配置。

## 独立环境模板

deploy/compose.ai.yml 提供社区版组成服务的独立模板，不应直接连接已有业务数据卷。复制 deploy/.env.example 为 deploy/.env，填写独立密码、JWT 密钥、已授权 MinIO 镜像及**真实兼容 RAG 服务地址**，并检查子网不冲突。

```bash
docker compose --env-file deploy/.env -f deploy/compose.ai.yml config --quiet
docker compose --env-file deploy/.env -f deploy/compose.ai.yml up -d --build
# web 两个应用完成构建后，启用前端镜像：
docker compose --env-file deploy/.env -f deploy/compose.ai.yml --profile frontend up -d --build
```

模板默认只绑定本机：后台 https://127.0.0.1:28443，API 127.0.0.1:28000，前台 Caddy 127.0.0.1:19880。对外提供服务时使用自己的域名、证书和访问策略。前台知识库配置的内部监听端口需与映射一致。

该模板不会下载商业版或替换 RAG。CT RAG 服务负责切片、Embedding、Rerank，并需要与本部署的 PostgreSQL/NATS 事件回调兼容；其供应及部署方法仍以所用上游版本的社区版发布包为准。AnyDoc 导入、机器人及其他外部集成按原部署配置。缺少这些服务不能把完整功能视为已验证。

## 本地模型与数据

继续使用原模型配置页配置 Ollama/OpenAI 兼容服务及 Embedding/Rerank，再选择知识整理任务模型，或显式复用聊天模型。未启用策略/数据传输许可时不会启动新增整理模型调用。离线部署应同时提供可离线的 RAG、模型和依赖，关闭 Sentry，检查其他原有外部服务。

## 运维与回退

任务在收件箱的处理任务页展示。预算暂停后调整预算并手动重试；failed 记录检查真实服务后重试；取消任务会阻止后续候选提交。索引任务完成只表示已投递，最终学习状态以原文档列表为准。

回退优先关闭策略并回退应用镜像，保留新增数据库对象；不要直接运行 down 迁移。已发布变更通过回滚建议和人工审核恢复。监控任务 pending/running 时间、失败与预算暂停数量、原向量任务状态和数据库空间。审计与知识正文应与业务数据一起备份。

实际执行的测试、环境限制及未完成验收见 ../development/progress.md，不以配置模板的存在代表完整部署已实测。
