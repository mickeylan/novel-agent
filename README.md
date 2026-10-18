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

OpenAI-compatible LLM 与 Embedding 的 `base_url` 可填写服务根地址或已包含 `/v1` 的地址；Embedding Provider 同样使用 `modelscope`、`openai`、`ollama`、`sglang`、`llama` 严格白名单，未知值不会静默回退到 OpenAI-compatible；端点拼接会规范化，不会产生重复的 `/v1/v1`。LLM Provider 使用严格白名单，LLM 工厂返回显式构造错误，不会从环境二次取值、不会把拼写错误静默回退为 OpenAI 客户端，也不会为本地 Provider 生成或发送 dummy key；可用 `LLM_BASE_URL` 通用覆盖，专用 `MODELSCOPE_BASE_URL`、`OPENAI_BASE_URL`、`OLLAMA_BASE_URL`、`SGLANG_BASE_URL`、`LLAMA_BASE_URL` 仅在 Provider 匹配时生效。

API Key 不允许直接写入 `.env` 或 YAML。LLM 与 Embedding 使用彼此独立的密钥文件，不会互相复用；本地/无密钥 Provider 会清除程序化 Config 中可能残留的内存 Key，密钥重新加载失败也不会继续保留旧 Key；Service 构造函数要求远程 LLM 及任何配置了 Embedding 密钥的客户端具备由成功文件读取设置的内部来源标记；仅伪造密钥文件路径和内存明文 Key 也会被拒绝，该标记无法由 YAML 或环境变量设置。创建仅包含密钥的独立文件，并配置路径：

```env
LLM_API_KEY_FILE=../secrets/modelscope.key
# 仅远程 Embedding 需要；当前本地端点可不配置
EMBEDDING_API_KEY_FILE=../secrets/embedding.key
```

相对路径以 `backend/config.yaml` 所在目录为基准。Embedding ModelScope 强制要求独立密钥文件；其他 OpenAI-compatible Embedding 仅在 `localhost`、loopback 或私网 IP 时可无密钥；公网端点必须配置独立密钥，配置后仅向该 Embedding 请求发送 Bearer，不会复用 LLM Key。任何携带 LLM 或 Embedding 凭据的 Base URL 都必须使用 HTTPS；无密钥 HTTP 也仅允许 `localhost`、loopback 或私网 IP 字面量（不通过 DNS 推断），公网主机名/公网 IP 必须使用 HTTPS；本地 Ollama/llama.cpp/OpenAI-compatible HTTP 端点保持兼容，LLM/Embedder 工厂和实际发送请求前都会再次拒绝 HTTP 凭据，并在底层直接构造器路径重新验证端点必须为最长 2048 字节的绝对 HTTP(S) URL、具有 host 且不含 userinfo/query/fragment；模型名必须为 1–512 字节有效 UTF-8 且无控制/格式字符，非空 API Key 必须是不超过 16 KiB 的有效 UTF-8 单一非空白 token，即使未来代码绕过统一 Service 直接调用底层构造器也不会发出请求。密钥文件不得提交到 Git；文件最多 16 KiB 且必须是有效 UTF-8，去除首尾空白后必须是单个不含任何 Unicode 空白、控制或格式字符的可打印 token；读取或关闭文件失败都会失败关闭，且错误不会回显文件路径、内容或底层错误。数据库密码也不提供代码/YAML 默认值，必须通过被忽略的本地 `.env` 中的 `DB_PASSWORD` 或其他进程环境变量提供；缺失时服务在连接数据库前拒绝启动。环境覆盖先在 Config 副本上原子应用，任一后续解析失败都不会留下前半段修改；所有显式设置的字符串环境变量（包括空字符串）都会覆盖 YAML，空必填值随后在数据库/Provider 初始化前失败，不会静默回退可能遗留的 YAML 值；所有已设置的数字环境变量都采用严格整数解析；空值或非整数会在数据库初始化前终止配置加载，不会静默回退默认值，错误只报告变量名、不回显变量内容。非秘密字符串配置会在使用前去除首尾空白，并按字段限制长度，拒绝 Unicode 控制/格式字符及非普通空格分隔符；`server.mode` 会在 Router 构造前实际应用到 Gin；LLM/Embedding Provider 与 Vector 索引类型使用明确枚举，Vector 与 Embedding 维度必须一致。当前检索只实现 pgvector cosine，因此 `vector.metric` 仅接受 `cosine`；当前安全日志只实现标准文本/info，`logging.level`/`format` 仅接受 `info`/`text`，未实现值会失败而不是静默忽略；数据库密码在 YAML、进程环境和本地 `.env` 路径中均不裁剪（普通前后空格属于密码内容），但限制为 4096 字节且禁止控制字符。LLM/Embedding Base URL 最长 2048 字节，禁止内嵌 URL 凭据、查询串或 fragment。本地 `.env` 最多 64 KiB，必须是有效 UTF-8；读取、关闭或 UTF-8 校验失败都会在应用任何变量前返回脱敏错误；变量名和赋值语法严格校验并拒绝重复键；整份文件验证通过后才应用，后续错误不会造成前半段配置部分生效，应用失败会按逆序回滚本次新增变量；若回滚本身失败则启动失败并返回独立的脱敏错误，不会误报为已恢复原子状态。进程中已设置的变量优先，错误和启动日志不回显文件路径或值。`CONFIG_PATH` 未设置时默认 `config.yaml`；显式空值、超过 4096 字节、控制/格式字符或异常 Unicode 空白会在读取前失败且不回显路径，不会静默回退默认文件。配置路径按原始字节精确使用，不会裁剪普通前后空格并意外打开另一个文件。YAML 配置文件路径和所有配置文本必须是有效 UTF-8；路径最长 4096 字节且禁止控制字符，配置文件必须存在，不存在时不会静默使用内置默认值；文件最大 1 MiB，打开、读取或解析错误不回显路径和配置内容。YAML 必须包含且仅包含一个非空顶层对象；空文件、仅注释、`null`、数组、空对象、未知/拼错字段、类型错误和多文档文件都会拒绝；配置 YAML 最多 64 层、20000 个语法节点，并禁用 anchor/alias、非标量映射键和任意层级重复键，避免有界文件通过引用展开、深层结构或先值/后值解析差异产生资源和配置歧义；环境覆盖后的非秘密配置会在读取 API Key 文件前完成完整验证；`Config.Validate` 在副本上规范化并仅在全部检查成功后提交，nil 或失败验证不会 panic 或部分修改调用方配置；随后才加载凭据，最后才初始化数据库；AI Service 构造函数会在配置副本上重新执行完整校验并拒绝 nil/无效配置，不修改调用方对象，也不会在依赖注入或配置旁路错误时 panic、读取 Provider 或构造客户端。

LLM/Embedding 底层客户端首先检查上游 context 取消，在 Prompt/批次扫描、凭据检查、options 解析、请求序列化和网络访问前返回取消或 deadline 错误；LLM 底层客户端随后统一要求 Prompt 为有效 UTF-8、非纯空白且最多 1,000,000 个 Unicode 字符，防止未来绕过 Handler 的内部调用造成无界内存；LLM 调用参数统一校验：温度必须是有限数且位于 `0–2`（YAML `.nan`/`.inf` 会在凭据读取前拒绝）、top_p `(0,1]`、输出 token `1–40000`、超时 `1–1200` 秒；默认超时为 300 秒，且每次调用的超时不会延长上游请求已有的更短 deadline。LLM HTTP 客户端不再设置冲突的总超时，单次调用上下文是唯一总时限；传输层仍限制连接/TLS 握手为 10 秒、空闲连接为 90 秒，并复用每个 Provider 最多 10 个空闲连接。LLM 与 Embedding 出站客户端不继承 `HTTP_PROXY`/`HTTPS_PROXY` 等环境代理并禁止自动跟随 HTTP 重定向，避免 Prompt、小说正文、Embedding 输入或 Bearer 凭据被 Provider 的 307/308 响应转发到非预期端点。重试仅用于网络瞬态错误、HTTP 408/429/5xx；重试次数硬限制为 0–5 次，单次退避（含 `Retry-After`）最多 30 秒；`Retry-After` 只接受唯一的正整数秒数或未来 HTTP 日期，超大值安全钳制，重复、负数、溢出、过去日期及畸形值均忽略，且等待可被 context 立即取消；认证、解析、空响应和调用超时等确定性失败不会重试。退避会响应请求取消，不会在客户端断开后继续等待。ModelScope 与 OpenAI-compatible 流式响应按标准 SSE `data:` 帧解析，支持 `[DONE]`，单帧限制 1 MiB、累计输出限制 16 MiB；非流式响应同样限制为 16 MiB，Provider Transport 禁用隐式 gzip 协商/解压，成功响应拒绝任何 `Content-Encoding` 和重复 `Content-Type`，响应大小始终按 Provider 原始正文计算；并使用按模式媒体类型白名单：非流式 LLM/Embedding 仅接受 `application/json` 或 `application/*+json`，流式 LLM 仅接受 `text/event-stream`，并分别限制单帧 1 MiB、累计原始流输入 16 MiB 与累计模型输出 16 MiB，避免空行、注释或无 choices 事件绕过输出上限；媒体类型参数只能为空或唯一 `charset=utf-8`，且 JSON 正文和每个 SSE 事件都必须包含有效 UTF-8 字节、单一顶层对象、无重复键，并限制为 64 层嵌套、单容器 10,000 项和总节点 20,000；从模型文本提取的 `GenerateJSON` 结构化输出使用字符串与转义感知的平衡对象扫描并复用同一契约，能正确处理字符串内花括号，同时拒绝重复键、多对象拼接和超复杂内容；LLM 非流式、Ollama 和 SSE 最终正文统一拒绝空字符串、纯 Unicode 空白及清洗后为空的 reasoning，同时保留有效正文原始空白；SSE 仅接受空行、注释和无前导空白的 `data:` 行，拒绝裸 JSON、空 data 以及 `event:`/`id:`/`retry:` 等控制字段，并要求明确收到唯一的 `data: [DONE]` 后继续有界读取至 EOF；终止帧后仅允许空行或注释，任何第二终止帧、控制字段或数据帧均拒绝；连接提前 EOF 时即使已有部分正文也视为失败，非流式响应必须恰好包含一个 choice，SSE 的非空 choice 事件也最多一个；提供 choice `index` 时仅接受 `0`，拒绝多候选或非零索引；非流式及 SSE 的 `finish_reason=length/content_filter` 同样拒绝，防止截断或被过滤内容进入候选稿，缺失、畸形或 HTML 等响应在解析前拒绝；Provider 错误正文最多有界读取 64 KiB 后立即丢弃，不存入错误对象，也不会进入错误字符串、Handler 日志或客户端响应；错误对象仅保留 Provider HTTP 状态码和有界 `Retry-After` 等稳定元数据。结构化输出只接受非空顶层 JSON 对象，支持 Markdown `json` 代码块和对象前后说明文字；顶层数组、空对象及非法 JSON 会显式报错。Writer 章节摘要还要求非空 `summary`、`emotional_arc`、`key_events` 和 `characters`。Critic 审核要求 0–10 分的总分/维度分、非空维度反馈和总结、合法问题严重度及非空建议。Architect 三幕结构要求九个非空结构字段；章节大纲要求非空标题/概要/场景/冲突/类型/钩子、非空事件和节拍数组，以及 100–20000 的整数目标字数。DataAgent 要求 `state_changes`、`new_entities`、`resource_changes`、`hooks`、`subplots`、`world_facts` 和 `warnings` 七个数组全部明确存在，每类最多 200 项；空数组表示模型明确判断无候选。字段类型或数组元素无效时整次预览失败，不返回伪造的零值结果。Embedding HTTP Client 不设置冲突的固定总超时，调用总时限服从上游 request context；连接/TLS/响应头/空闲连接阶段继续有界。Embedding 调用限制为每批 1–128 条、总计最多 100000 个有效 UTF-8 Unicode 字符；每条输入必须非纯 Unicode 空白，字符计数使用无额外 rune 切片分配的扫描，响应限制 16 MiB，并拒绝数量不匹配、空向量、批内维度不一致、NaN/Inf 或转换到 float32 时溢出的结果；OpenAI 标准 `index` 存在时按索引恢复输入顺序并拒绝缺失、重复、负数或越界索引，全部缺少 index 时保留本地兼容服务的响应顺序；共享 `EmbedderClient` 还会在单条和批量返回处强制匹配已配置的 `embedding.dimension`（当前 1024），即使未来调用绕过 Retrieval 也不会接受稳定但错误维度的向量；异常向量不会进入 pgvector 查询。HTTP Server 使用 10 秒请求头、30 秒请求体读取、20 分钟响应写入及 120 秒空闲超时；收到退出信号后最多等待 30 秒优雅关闭。

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
- `POST /api/v1/projects`（标题 1–500 字符；题材最多 20 项、每项 1–100 字符，去除首尾空白并拒绝规范化后重复项；世界设定与前提各最多 12000 字符，写库前校验）
- `GET /api/v1/projects/:id`
- `PUT /api/v1/projects/:id`（与创建相同的字段边界；状态 1–100 字符；版本、字段和空更新校验均先于数据库访问）
- `GET /api/v1/projects/:id/export?format=txt|md`

### 角色

- `GET /api/v1/projects/:id/characters`
- `POST /api/v1/projects/:id/characters`（姓名 1–500 字符；特征最多 100 项并去除首尾空白；关系最多 200 项并限制键值长度、规范化键值且拒绝规范化后重复键；摘要与状态各最多 12000 字符，写库前校验）
- `GET /api/v1/projects/:id/characters/:character_id`
- `PUT /api/v1/projects/:id/characters/:character_id`（与创建相同的字段边界，写库前校验）

### 章节与创作

- `GET /api/v1/projects/:id/chapters`
- `POST /api/v1/projects/:id/chapters`（章节号 1–1000000；标题最多 500、大纲最多 12000、正文最多 100000 个 Unicode 字符，写库前校验）
- `GET /api/v1/projects/:id/chapters/:num`
- `PUT /api/v1/projects/:id/chapters/:num`（与创建相同的标题/大纲/正文边界，写库前校验）
- `GET /api/v1/projects/:id/chapters/:num/prompt`
- `POST /api/v1/projects/:id/chapters/:num/prompt-preview`（组合项目上下文、前文文风、可选场景计划和 RAG 的完整 Writer Prompt，不调用 LLM）
- `POST /api/v1/projects/:id/chapters/:num/scene-plan-preview`（按关键事件或大纲拆分最多 10 个场景并分配字数）
- `GET /api/v1/projects/:id/chapters/:num/context-preview`（项目设定、角色与最近章节，只读）
- `POST /api/v1/projects/:id/generate-outline`（预览；故事前提 1–12000 字符、题材最多 200 字符、可选目标章节数 1–100，均在模型调用前校验）
- `POST /api/v1/projects/:id/chapters/generate-outlines`（批量预览，最多 20 章）
- `POST /api/v1/projects/:id/chapters/:num/write`（预览；要求非空大纲，目标字数 100–20000，大纲最多 12000 字符，文风指南最多 4000 字符，均在数据库或模型调用前校验；模型输出经清理后为空会失败关闭，不返回空候选稿）
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
- `POST /api/v1/projects/:id/chapters/:num/revise-preview`（AI 修订候选稿，不自动保存；修订后快速评分失败时保留候选稿，并以 `candidate_score_status: "unavailable"` 明确标记，不静默伪装为已评分）
- `POST /api/v1/projects/:id/audit`（兼容接口）
- `POST /api/v1/projects/:id/polish`（兼容接口；Editor 输出去空白后为空会失败关闭，不返回空润色稿）

### 检索

- `POST /api/v1/search/vector`
- `POST /api/v1/search/keyword`
- `POST /api/v1/search/hybrid`
- `POST /api/v1/projects/:id/rag-context-preview`（只读检索证据与 Prompt 上下文）

## 数据库安全

前端普通 CRUD/规则预览统一使用 30 秒超时；明确会调用 LLM/Embedding 的预览使用最高 20 分钟的显式覆盖，与服务端长响应边界一致，禁止无界等待。客户端超时时会提示服务端可能仍在处理，并保留请求 ID 供日志关联。高成本 LLM/Embedding 路由共享并发上限，默认 4、可通过 `server.max_concurrent_ai` 或 `API_MAX_CONCURRENT_AI` 配置为 1–64；容量耗尽时立即返回 HTTP 429、`code: "ai_capacity_exceeded"` 和 `Retry-After: 1`，不会排队占用数据库或 Provider 连接。普通 CRUD、健康检查、预算预览和纯规则分析不占用该配额。HTTP 请求头默认限制为 64 KiB，可通过 `server.max_header_bytes` 或 `API_MAX_HEADER_BYTES` 在 8 KiB–1 MiB 范围内配置；超限请求由 Go HTTP Server 在进入 Gin、数据库或 Provider 前拒绝。所有 `/api/v1` 请求要求 `X-Request-ID`、`Content-Type`、`Content-Encoding` 最多各出现一次，重复关键请求头会在解析前返回 `ambiguous_request_header`；同时拒绝任何非空 `Content-Encoding`（包括 gzip/br/deflate/identity），避免代理解压与应用原始字节解析不一致；POST/PUT/PATCH 仅接受 `application/json`，或唯一参数为 `charset=utf-8`（大小写不敏感）；其他 charset、未知或附加参数均返回 415，JSON 请求体必须是有效 UTF-8 且恰好包含一个顶层对象，非法字节不会被解码器静默替换为 `U+FFFD`；空正文、`null`、数组、标量、拼接或尾随第二个 JSON 值均在 Handler 前拒绝；DTO 严格拒绝未知或拼错字段，所有 Handler 的 JSON 绑定失败统一返回 `invalid_request_body`，不回显 Go 类型、内部字段名或底层解码器错误；且递归拒绝顶层、嵌套对象及数组对象中的重复键，JSON 嵌套深度最多 64 层、单个对象/数组最多 10000 项且总值节点最多 20000，避免安全参数、预算或乐观锁字段因拼写错误被静默忽略；其他媒体类型在读取正文前返回 HTTP 415 和 `code: "unsupported_media_type"`；这也使浏览器跨域简单表单请求无法绕过 JSON 契约。所有非 GET/HEAD 请求体统一限制为 1 MiB；超限返回 HTTP 413 和 `code: "request_body_too_large"`。当前 JSON 创作接口不得绕过该限制，未来文件导入需使用独立且显式受限的上传设计。`GET /api/v1/health` 在数据库不可用时返回 HTTP 503、`status: "unhealthy"` 和 `database: "unavailable"`，不会向未认证客户端暴露驱动、主机或连接错误原文。所有 API 响应设置 `Cache-Control: no-store`、`Pragma: no-cache`、`X-Content-Type-Options: nosniff`、`Referrer-Policy: no-referrer` 和 `X-Frame-Options: DENY`，且不启用跨域开放；小说正文、Prompt、RAG 证据和错误响应不会被共享缓存保存。前端会为每个 API 请求生成 `X-Request-ID` UUID；已有合法显式 ID 不会被覆盖。服务端校验、规范化并回传同一 UUID，非法值会替换，避免日志注入，使浏览器报错可关联到服务端请求日志。所有 `:project_id`/`:character_id` 路径参数在 Handler、数据库或模型调用前验证为标准连字符格式的非零 UUID（大小写均可），无连字符、花括号/URN 或 nil UUID 均拒绝，非法值返回 `invalid_project_id`/`invalid_character_id`；所有路径或 JSON 正文中的 `chapter_number` 统一验证为 1–1000000 的整数，非法值返回 JSON HTTP 400 / `invalid_chapter_number` 和请求 ID，不再静默转换为 `0/-1`。所有已注册的 `/api/v1` GET 路由拒绝任何请求正文（包括未知长度或 chunked 正文），避免代理与应用对读取请求产生歧义；不支持的 HEAD 继续按既有 405 契约处理。所有 `/api/v1` 路由默认禁止查询参数；仅 Writer Context 的 `max_runes`、Writer Prompt 的 `target_word_count` 和导出的 `format` 通过逐路由白名单开放，并在数据库访问前校验。未知键、同名重复参数或畸形查询串均返回 `invalid_query_parameter`；错误不反射原始查询值。未知 API 路径返回 JSON HTTP 404 / `route_not_found`，已知路径的错误方法（包括所有未注册 DELETE）返回 JSON HTTP 405 / `method_not_allowed`；两者均带安全头和请求 ID。所有 Handler 的内部失败（包括上下文组装、RAG、风格样本和 Prompt 预览）及未预期 panic 均统一返回 HTTP 500 和 `code: "internal_error"`，且 `error_id`/`request_id` 与响应头一致；panic 值不会写入响应或日志，只记录类型、栈和请求 ID，AI 并发槽也会可靠释放。工作流步骤和可选 DataAgent 失败只返回 `step_failed`/`data_extract_failed` 稳定码并保留候选稿，不返回 Provider、解码或正文错误详情；Handler 日志不记录任意 Repository、Provider 或编排错误文本，只记录请求 ID、路由模板、稳定操作名、取消/超时/内部错误类别和 Go 错误类型；Provider 响应正文、数据库细节及小说内容不会进入日志。Gin 默认访问日志与默认 Recovery 已禁用；API 安全访问日志只记录请求 ID、HTTP 方法、路由模板、状态码和耗时，不记录实际资源 ID、查询字符串、请求体或请求头。前端统一校验并展示稳定错误码、合法请求 UUID 和数字形式的 `Retry-After`：429 会提示建议等待时间，409 会提示保留草稿并对比版本，500 可凭请求 ID 排障；畸形关联字段和过长错误文本不会直接展示。

PostgreSQL 连接池默认最多 25 个连接、保留 5 个空闲连接，连接最长存活 30 分钟、最长空闲 5 分钟；上限分别校验为 200 个连接、24 小时生命周期，且空闲连接/时间不得超过对应总上限。连接建立默认限制为 10 秒，单条 SQL 默认限制为 15 秒；可通过 `DB_CONNECT_TIMEOUT_SECONDS`（1–60）和 `DB_STATEMENT_TIMEOUT_MS`（100–120000）调整。GORM SQL 日志已关闭，避免查询参数中的未发布正文、Prompt 或凭据进入日志；Repository 错误仍通过请求 ID 关联。数据库可用性同时要求 GORM 句柄、配置与底层连接池均存在，畸形的非 nil 句柄会在 Ping/Repository/检索前失败而不是 panic；`database.Init` 会在副本上独立重验并规范化完整数据库配置，使用规范化副本构造 DSN 且不修改调用方；数据库密码保持原始字节不裁剪，非法替换配置在关闭旧池或调用驱动前失败；重复初始化会先摘除并关闭旧池，旧池重置失败时不会打开替换池；新句柄只有在结构校验、底层池获取和全部连接池参数配置成功后才会一次性发布到全局，nil/畸形驱动结果不会短暂暴露给请求；驱动同时返回候选池和错误时也会尽力关闭该候选池，关闭错误不会覆盖稳定初始化错误或泄露底层详情；关闭数据库池时先摘除全局句柄，并直接验证/关闭底层 ConnPool，Config 或池缺失不会触发 GORM 内部 panic；即使底层池关闭失败，Repository 也不会继续把已关闭或异常句柄当作可用连接；启动入口仅记录稳定的关闭阶段信息，不输出底层错误。数据库连接串使用标准 PostgreSQL URL 编码，用户名、密码、IPv6 主机、库名和运行参数不会因空格、引号或分隔符越界；`sslmode` 采用固定白名单，`search_path` 仅允许最多 16 个、每个最长 63 字节的未引用 PostgreSQL 标识符。数据库驱动初始化错误通过可注入失败测试确认只返回稳定阶段信息、清空全局句柄且不保留可能含密码的底层 DSN 错误链，启动日志也不输出数据库用户、主机、端口、库名、LLM/Embedding Provider、模型名、Base URL 或密钥文件路径，仅报告 AI 服务和数据库连接池已配置。健康检查 Ping 使用请求上下文和 3 秒 deadline，只读 Schema 检查使用请求上下文和 5 秒 deadline；客户端断开会取消对应查询。Project、Chapter、Character 与检索 Repository/Service 已传播 HTTP 请求取消上下文；创建操作返回缺失/畸形/零 UUID、创建或乐观锁更新后的资源重新读取错误或异常 `(nil,nil)`，都会显式返回关联 HTTP 500、`write_result_unavailable` 和 `retry_safe:false`；角色创建也会重载数据库结果并返回真实 `updated_at`，不再忽略错误或因 nil 结果触发 panic；前端提示先刷新核对服务器数据、禁止直接重试，避免重复创建或覆盖；写作上下文、人物状态、前文章节、Embedding、pgvector/关键词/混合查询及结果迭代均可取消；所有写作、审核、工作流、连贯性、修订、评估和 Prompt 预览的前文章节读取仅将明确不存在视为空上下文；取消或数据库读取失败会走安全 500，不再静默继续调用规则分析或模型；项目、角色、章节及写作上下文读取也只有 `record not found` 映射为 404，其他 Repository 故障不再伪装成资源不存在；所有项目、角色、章节、大纲及预览读取路径在解引用前验证 Repository 结果；若违反契约返回 `(nil, nil)`，统一视为安全 500，不返回伪 404 且不触发 nil 解引用，尾部截取统一按 Unicode 字符执行。健康 Ping 使用 3 秒、只读 Schema 检查使用 5 秒 deadline；所有当前数据库访问域已完成上下文传播。本项目直接复用已有 Python 后端数据库，不包含也不执行初始化或迁移脚本。服务启动只配置惰性连接池，不主动查询或执行 `Ping`；真实连通性由 `/api/v1/health` 按请求检查，且不会调用 `AutoMigrate`。所有 DELETE API 和前端删除入口均已禁用；数据库结构只能通过只读查询核对。AI 生成与润色接口只返回预览；候选稿“放入编辑器”不会自动保存，页面会持续标记未保存状态，切换章节或离开页面前会确认。只有用户单独点击保存才会更新正文。项目、角色、章节和两类大纲更新必须携带非零的读取时 `updated_at`；缺失或显式零时间统一在 Repository 前返回 `invalid_update_version`，不依赖绑定器的 `required` 行为；所有更新先完成 JSON 契约、版本字段及空更新校验，再访问 Repository；人物关系 JSON 的编码、Repository 类型转换及数据库读取解码错误均显式失败；新建角色未提供关系时写入 `{}` 而非 JSON `null`，历史 NULL 读取语义保持兼容；版本过期时返回 HTTP 409 和 `loaded_updated_at`（仅表示提交时基于的已加载版本），不会把该旧值误称为服务器当前版本；当前草稿会保留且不会自动重载或重试；用户可显式复制或下载 UTF-8 本地草稿、只读对比服务器正文与本地草稿，或在二次确认后加载服务器版本。只读对比不会更新 Store 或本地 `updated_at`；加载前会重新读取并核对对比时返回的服务器 `updated_at`，如版本再次变化则拒绝加载并要求重新对比，避免基于过期差异覆盖新数据。
