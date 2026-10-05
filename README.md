# Novel Agent Go

AI 驱动的小说创作助手 - Go 版本

## 支持的 LLM Provider

| Provider | 说明 | 配置 |
|----------|------|------|
| `modelscope` | 通义千问 API | 需要 `MODELSCOPE_API_KEY` |
| `ollama` | 本地 Ollama 服务 | `OLLAMA_BASE_URL` |
| `sglang` | 本地 SGLang 服务 | `SGLANG_BASE_URL` |
| `llama` | 本地 llama.cpp server | `LLAMA_BASE_URL` |
| `openai` | OpenAI API | `OPENAI_API_KEY` |

## 快速开始

```bash
cd novel-agent

# 复制环境配置
cp .env.example .env
# 编辑 .env 设置 LLM_PROVIDER

# 运行
./novel-agent.exe
```

## 配置示例

### 使用通义千问 (ModelScope)
```bash
LLM_PROVIDER=modelscope
MODELSCOPE_API_KEY=your-api-key
MODELSCOPE_BASE_URL=https://coding.dashscope.aliyuncs.com/v1
LLM_MODEL=qwen3.5-plus
```

### 使用本地 Ollama
```bash
LLM_PROVIDER=ollama
OLLAMA_BASE_URL=http://localhost:11434
LLM_MODEL=llama3.1:8b
```

### 使用本地 llama.cpp
```bash
LLM_PROVIDER=llama
LLAMA_BASE_URL=http://10.0.0.200:8080
LLM_MODEL=Qwen3.5-9B-UD-Q4_K_XL.gguf
```

## API 端点

### 项目管理
- `POST /api/v1/projects` - 创建项目
- `GET /api/v1/projects` - 列出项目
- `GET /api/v1/projects/:id` - 获取项目
- `DELETE /api/v1/projects/:id` - 删除项目

### 角色管理
- `POST /api/v1/projects/:id/characters` - 添加角色
- `GET /api/v1/projects/:id/characters` - 列出角色

### 章节管理
- `POST /api/v1/projects/:id/chapters` - 创建章节
- `GET /api/v1/projects/:id/chapters` - 列出章节
- `GET /api/v1/projects/:id/chapters/:num` - 获取章节
- `PUT /api/v1/projects/:id/chapters/:num` - 更新章节

### 创作功能
- `POST /api/v1/projects/:id/generate-outline` - 生成大纲
- `POST /api/v1/projects/:id/chapters/:num/write` - 写作章节
- `POST /api/v1/projects/:id/audit` - 审核章节
- `POST /api/v1/projects/:id/polish` - 润色章节

### 搜索
- `POST /api/v1/search/vector` - 向量搜索
- `POST /api/v1/search/keyword` - 关键词搜索

### 健康检查
- `GET /api/v1/health` - 健康检查

## 开发

```bash
# 编译
go build -o novel-agent.exe ./cmd/server

# 运行
./novel-agent.exe
```
