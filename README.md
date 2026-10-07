# Novel Agent

AI 驱动的小说创作助手，包含 Go 后端和 Vue 3 前端。

## 目录

- `backend/`：Gin、PostgreSQL/pgvector、LLM、Embedding、RAG 与 Agent 服务
- `frontend/`：Vue 3、Vite、Pinia、Vue Router

## LLM Provider

| Provider | 配置 |
|---|---|
| `modelscope` | OpenAI-compatible；需要 API Key 文件 |
| `openai` | OpenAI-compatible；需要 API Key 文件 |
| `ollama` | `OLLAMA_BASE_URL` |
| `sglang` | `SGLANG_BASE_URL` |
| `llama` | llama.cpp OpenAI-compatible；`LLAMA_BASE_URL` |

OpenAI-compatible LLM 与 Embedding 的 `base_url` 可填写服务根地址或已包含 `/v1` 的地址；端点拼接会规范化，不会产生重复的 `/v1/v1`。LLM Provider 使用严格白名单，不会将拼写错误静默回退为带 dummy key 的 OpenAI 客户端；可用 `LLM_BASE_URL` 通用覆盖，专用 `MODELSCOPE_BASE_URL`、`OPENAI_BASE_URL`、`OLLAMA_BASE_URL`、`SGLANG_BASE_URL`、`LLAMA_BASE_URL` 仅在 Provider 匹配时生效。

API Key 不允许直接写入 `.env` 或 YAML。创建仅包含密钥的独立文件，并配置路径：

```env
LLM_API_KEY_FILE=../secrets/modelscope.key
```

相对路径以 `backend/config.yaml` 所在目录为基准。密钥文件不得提交到 Git。数据库密码也不提供代码/YAML 默认值，必须通过被忽略的本地 `.env` 中的 `DB_PASSWORD` 或其他进程环境变量提供；缺失时服务在连接数据库前拒绝启动。所有已设置的数字环境变量都采用严格整数解析；空值或非整数会在数据库初始化前终止配置加载，不会静默回退默认值，错误只报告变量名、不回显变量内容。YAML 配置同样采用严格字段解析：未知/拼错字段、类型错误和多文档文件都会在数据库初始化前拒绝，错误使用稳定描述且不回显配置值。

LLM 调用参数统一校验：温度 `0–2`、top_p `(0,1]`、输出 token `1–40000`、超时 `1–1200` 秒；默认超时为 300 秒，且每次调用的超时不会延长上游请求已有的更短 deadline。LLM HTTP 客户端不再设置冲突的总超时，单次调用上下文是唯一总时限；传输层仍限制连接/TLS 握手为 10 秒、空闲连接为 90 秒，并复用每个 Provider 最多 10 个空闲连接。重试仅用于网络瞬态错误、HTTP 408/429/5xx，并尊重较长的 `Retry-After`；认证、解析、空响应和调用超时等确定性失败不会重试。退避会响应请求取消，不会在客户端断开后继续等待。ModelScope 与 OpenAI-compatible 流式响应按标准 SSE `data:` 帧解析，支持 `[DONE]`，单帧限制 1 MiB、累计输出限制 16 MiB；非流式响应同样限制为 16 MiB，Provider 错误正文限制为 64 KiB。结构化输出只接受非空顶层 JSON 对象，支持 Markdown `json` 代码块和对象前后说明文字；顶层数组、空对象及非法 JSON 会显式报错。Writer 章节摘要还要求非空 `summary`、`emotional_arc`、`key_events` 和 `characters`。Critic 审核要求 0–10 分的总分/维度分、非空维度反馈和总结、合法问题严重度及非空建议。Architect 三幕结构要求九个非空结构字段；章节大纲要求非空标题/概要/场景/冲突/类型/钩子、非空事件和节拍数组，以及 100–20000 的整数目标字数。DataAgent 要求 `state_changes`、`new_entities`、`resource_changes`、`hooks`、`subplots`、`world_facts` 和 `warnings` 七个数组全部明确存在，每类最多 200 项；空数组表示模型明确判断无候选。字段类型或数组元素无效时整次预览失败，不返回伪造的零值结果。Embedding 调用限制为每批 1–128 条、总计最多 100000 个 Unicode 字符，响应限制 16 MiB，并拒绝数量不匹配、空向量或批内维度不一致的结果。HTTP Server 使用 10 秒请求头、30 秒请求体读取、20 分钟响应写入及 120 秒空闲超时；收到退出信号后最多等待 30 秒优雅关闭。

## 启动后端

```bash
cd backend
go mod download
go run ./cmd/server
```

构建：

```bash
cd backend
go build -o novel-agent.exe ./cmd/server
```

默认 API 地址：`http://127.0.0.1:8080/api/v1`。

## 启动前端

```bash
cd frontend
npm install
npm run dev
```

默认前端地址：`http://127.0.0.1:5173`，Vite 将 `/api` 代理到 `http://127.0.0.1:8080`。

## 主要 API

### 系统检查

- `GET /api/v1/health`
- `GET /api/v1/schema/compatibility`（只读查询 PostgreSQL 系统目录）

### 项目

- `GET /api/v1/projects`
- `POST /api/v1/projects`
- `GET /api/v1/projects/:id`
- `PUT /api/v1/projects/:id`
- `GET /api/v1/projects/:id/export?format=txt|md`

### 角色

- `GET /api/v1/projects/:id/characters`
- `POST /api/v1/projects/:id/characters`
- `GET /api/v1/projects/:id/characters/:character_id`
- `PUT /api/v1/projects/:id/characters/:character_id`

### 章节与创作

- `GET /api/v1/projects/:id/chapters`
- `POST /api/v1/projects/:id/chapters`
- `GET /api/v1/projects/:id/chapters/:num`
- `PUT /api/v1/projects/:id/chapters/:num`
- `GET /api/v1/projects/:id/chapters/:num/prompt`
- `POST /api/v1/projects/:id/chapters/:num/prompt-preview`（组合项目上下文、前文文风、可选场景计划和 RAG 的完整 Writer Prompt，不调用 LLM）
- `POST /api/v1/projects/:id/chapters/:num/scene-plan-preview`（按关键事件或大纲拆分最多 10 个场景并分配字数）
- `GET /api/v1/projects/:id/chapters/:num/context-preview`（项目设定、角色与最近章节，只读）
- `POST /api/v1/projects/:id/generate-outline`（预览）
- `POST /api/v1/projects/:id/chapters/generate-outlines`（批量预览，最多 20 章）
- `POST /api/v1/projects/:id/chapters/:num/write`（预览；目标字数 100–20000，大纲最多 12000 字符，文风指南最多 4000 字符）
- `POST /api/v1/workflow-budget-preview`（纯计算最坏情况输出 Token 预算和逐步骤拆分，不读取项目、数据库、RAG 或调用模型）
- `POST /api/v1/projects/:id/chapters/:num/workflow-preview`（自动装配上下文和可选约束，对最终候选稿执行多维检查；支持 `max_total_tokens`，写作、审核、可选一次修订/复审和 DataAgent 在数据库读取及首个模型调用前统一预留，最高 62000；全程不落库）
- `POST /api/v1/projects/:id/chapters/:num/continue-preview`（续写指令 1–4000 字符，目标字数 100–10000；校验通过后才读取章节或调用模型）
- `POST /api/v1/projects/:id/chapters/:num/rewrite-preview`（重写指令 1–4000 字符；校验通过后才读取章节或调用模型）
- `GET /api/v1/projects/:id/chapters/:num/summary-preview`
- `POST /api/v1/projects/:id/evaluate`
- `POST /api/v1/projects/:id/chapters/:num/polish`（预览）
- `POST /api/v1/projects/:id/chapters/:num/validate`（纯规则连贯性检查）
- `POST /api/v1/projects/:id/chapters/:num/diff-preview`（Unicode 安全的候选稿变更区与增删统计；允许显式空候选稿，缺失字段与越界参数会在数据库读取前拒绝）
- `POST /api/v1/projects/:id/chapters/:num/scene-coverage`（场景覆盖、顺序与篇幅偏差的规则检查）
- `POST /api/v1/projects/:id/chapters/:num/aigc-analysis`（确定性规则分析，仅提供编辑建议）
- `POST /api/v1/projects/:id/chapters/:num/style-analysis`（句长、段长、词汇、对话、连接词与标点风格；可与前 5 个有正文的章节比较并生成只读写作约束）
- `POST /api/v1/projects/:id/chapters/:num/data-changes-preview`（人物、资源、伏笔、支线与世界事实候选；不写库）
- `GET /api/v1/projects/:id/chapters/:num/reader-analysis`（纯计算读者视角分析）
- `POST /api/v1/projects/:id/chapters/:num/revise-preview`（AI 修订候选稿，不自动保存）
- `POST /api/v1/projects/:id/audit`（兼容接口）
- `POST /api/v1/projects/:id/polish`（兼容接口）

### 检索

- `POST /api/v1/search/vector`
- `POST /api/v1/search/keyword`
- `POST /api/v1/search/hybrid`
- `POST /api/v1/projects/:id/rag-context-preview`（只读检索证据与 Prompt 上下文）

## 数据库安全

前端普通 CRUD/规则预览统一使用 30 秒超时；明确会调用 LLM/Embedding 的预览使用最高 20 分钟的显式覆盖，与服务端长响应边界一致，禁止无界等待。客户端超时时会提示服务端可能仍在处理，并保留请求 ID 供日志关联。高成本 LLM/Embedding 路由共享并发上限，默认 4、可通过 `server.max_concurrent_ai` 或 `API_MAX_CONCURRENT_AI` 配置为 1–64；容量耗尽时立即返回 HTTP 429、`code: "ai_capacity_exceeded"` 和 `Retry-After: 1`，不会排队占用数据库或 Provider 连接。普通 CRUD、健康检查、预算预览和纯规则分析不占用该配额。HTTP 请求头默认限制为 64 KiB，可通过 `server.max_header_bytes` 或 `API_MAX_HEADER_BYTES` 在 8 KiB–1 MiB 范围内配置；超限请求由 Go HTTP Server 在进入 Gin、数据库或 Provider 前拒绝。所有 `/api/v1` POST/PUT/PATCH 仅接受 `application/json`（允许 `charset` 参数），JSON 请求体必须恰好包含一个顶层对象，空正文、`null`、数组、标量、拼接或尾随第二个 JSON 值均在 Handler 前拒绝；DTO 严格拒绝未知或拼错字段，且递归拒绝顶层、嵌套对象及数组对象中的重复键，JSON 嵌套深度最多 64 层，避免安全参数、预算或乐观锁字段因拼写错误被静默忽略；其他媒体类型在读取正文前返回 HTTP 415 和 `code: "unsupported_media_type"`；这也使浏览器跨域简单表单请求无法绕过 JSON 契约。所有非 GET/HEAD 请求体统一限制为 1 MiB；超限返回 HTTP 413 和 `code: "request_body_too_large"`。当前 JSON 创作接口不得绕过该限制，未来文件导入需使用独立且显式受限的上传设计。`GET /api/v1/health` 在数据库不可用时返回 HTTP 503、`status: "unhealthy"` 和 `database: "unavailable"`，不会向未认证客户端暴露驱动、主机或连接错误原文。所有 API 响应设置 `Cache-Control: no-store`、`Pragma: no-cache`、`X-Content-Type-Options: nosniff`、`Referrer-Policy: no-referrer` 和 `X-Frame-Options: DENY`，且不启用跨域开放；小说正文、Prompt、RAG 证据和错误响应不会被共享缓存保存。前端会为每个 API 请求生成 `X-Request-ID` UUID；已有合法显式 ID 不会被覆盖。服务端校验、规范化并回传同一 UUID，非法值会替换，避免日志注入，使浏览器报错可关联到服务端请求日志。所有 `:project_id`/`:character_id` 路径参数在 Handler、数据库或模型调用前验证为标准连字符格式的非零 UUID（大小写均可），无连字符、花括号/URN 或 nil UUID 均拒绝，非法值返回 `invalid_project_id`/`invalid_character_id`；所有含 `:chapter_number` 的 API 路径统一验证为 1–1000000 的整数，非法值返回 JSON HTTP 400 / `invalid_chapter_number` 和请求 ID，不再静默转换为 `0/-1`。未知 API 路径返回 JSON HTTP 404 / `route_not_found`，已知路径的错误方法（包括所有未注册 DELETE）返回 JSON HTTP 405 / `method_not_allowed`；两者均带安全头和请求 ID。普通内部错误及未预期 panic 均统一返回 HTTP 500 和 `code: "internal_error"`，且 `error_id`/`request_id` 与响应头一致；panic 值不会写入响应或日志，只记录类型、栈和请求 ID，AI 并发槽也会可靠释放。详细 Repository、Provider 和编排错误仅写入带同一请求 ID 的服务端日志。Gin 默认访问日志与默认 Recovery 已禁用；API 安全访问日志只记录请求 ID、HTTP 方法、路由模板、状态码和耗时，不记录实际资源 ID、查询字符串、请求体或请求头。前端统一校验并展示稳定错误码、合法请求 UUID 和数字形式的 `Retry-After`：429 会提示建议等待时间，409 会提示保留草稿并对比版本，500 可凭请求 ID 排障；畸形关联字段和过长错误文本不会直接展示。

PostgreSQL 连接池默认最多 25 个连接、保留 5 个空闲连接，连接最长存活 30 分钟、最长空闲 5 分钟；上限分别校验为 200 个连接、24 小时生命周期，且空闲连接/时间不得超过对应总上限。连接建立默认限制为 10 秒，单条 SQL 默认限制为 15 秒；可通过 `DB_CONNECT_TIMEOUT_SECONDS`（1–60）和 `DB_STATEMENT_TIMEOUT_MS`（100–120000）调整。GORM SQL 日志已关闭，避免查询参数中的未发布正文、Prompt 或凭据进入日志；Repository 错误仍通过请求 ID 关联。数据库连接串使用标准 PostgreSQL URL 编码，用户名、密码、IPv6 主机、库名和运行参数不会因空格、引号或分隔符越界；`sslmode` 采用固定白名单，`search_path` 仅允许最多 16 个、每个最长 63 字节的未引用 PostgreSQL 标识符。数据库驱动初始化错误不携带可能含密码的底层 DSN 错误链，启动日志也不输出数据库用户、主机、端口或库名。健康检查 Ping 使用请求上下文和 3 秒 deadline，只读 Schema 检查使用请求上下文和 5 秒 deadline；客户端断开会取消对应查询。Project、Chapter、Character 与检索 Repository/Service 已传播 HTTP 请求取消上下文；创建或乐观锁更新后的资源重新读取错误会显式返回关联 HTTP 500、`write_result_unavailable` 和 `retry_safe:false`，不再忽略错误或因 nil 结果触发 panic；前端提示先刷新核对服务器数据、禁止直接重试，避免重复创建或覆盖；写作上下文、人物状态、前文章节、Embedding、pgvector/关键词/混合查询及结果迭代均可取消。健康 Ping 使用 3 秒、只读 Schema 检查使用 5 秒 deadline；所有当前数据库访问域已完成上下文传播。本项目直接复用已有 Python 后端数据库，不包含也不执行初始化或迁移脚本。服务启动只配置惰性连接池，不主动查询或执行 `Ping`；真实连通性由 `/api/v1/health` 按请求检查，且不会调用 `AutoMigrate`。所有 DELETE API 和前端删除入口均已禁用；数据库结构只能通过只读查询核对。AI 生成与润色接口只返回预览；候选稿“放入编辑器”不会自动保存，页面会持续标记未保存状态，切换章节或离开页面前会确认。只有用户单独点击保存才会更新正文。项目、角色、章节和大纲更新必须携带读取时的 `updated_at`；版本过期时返回 HTTP 409，当前草稿会保留且不会自动重载或重试；用户可显式复制或下载 UTF-8 本地草稿、只读对比服务器正文与本地草稿，或在二次确认后加载服务器版本。只读对比不会更新 Store 或本地 `updated_at`；加载前会重新读取并核对对比时返回的服务器 `updated_at`，如版本再次变化则拒绝加载并要求重新对比，避免基于过期差异覆盖新数据。
