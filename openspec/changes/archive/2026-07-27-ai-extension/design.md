## Context

AI Extension Plane 是 T3 可选能力，可独立启停。AI-004 要求 Copilot/AIOps 不得绕过 Operation 路径，AI-005 要求高风险自动化有限制。

## Goals / Non-Goals

**Goals:**
- `cmd/ai-extension/` 服务骨架，支持独立启停
- 写操作检测：所有 AI 发出的写操作必须经过 Operation
- 高风险自动化熔断：冷却期、回滚、效果验证

**Non-Goals:**
- AI Gateway 数据面实现（AI-003 仅定义接口）
- 模型资源模型 CRD 生成（AI-002 仅定义模型）

## Decisions

### Decision 1: AI Extension 为独立服务
独立进程 `cmd/ai-extension/`，不编译进 Core。停用时不影响 T0/T1 平台。

### Decision 2: 写操作旁路检测在 API 层
Platform API 添加中间件检查请求来源，AI 来源的写操作必须携带 Operation 引用。

## Implementation

### AI Extension 服务骨架
- `cmd/ai-extension/main.go`：健康检查端点 + 空服务
- `cmd/ai-extension/go.mod`：独立模块

### 写操作旁路检测
在 `cmd/platform-api/internal/api/server.go` 中添加 `handleAICheck` 中间件，检查 AI 来源请求是否包含 `operationId` 引用。