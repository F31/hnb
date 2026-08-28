# 配额体系 API 规范

## 概述

配额体系为 HNB 平台提供租户级和工作空间级的资源额度管理。支持两级分配模型：

```
租户配额（总量） → 工作空间配额（子分配）
```

## 数据模型

### `quota` JSON 结构

```json
{
  "cpu": 0,
  "memory": 0,
  "storage": 0,
  "vgpu": 0,
  "vram": 0,
  "gpu": 0
}
```

| 字段 | 类型 | 单位 | 说明 |
|------|------|------|------|
| cpu | int | 核 | CPU 核心数 |
| memory | int | Gi | 内存大小 |
| storage | int | Gi | 存储大小 |
| vgpu | int | % | 虚拟 GPU 百分比 |
| vram | int | MB | 虚拟显存 |
| gpu | int | 块 | GPU 数量 |

## API 端点

### 1. 租户级配额

#### 1.1 获取租户配额

```
GET /api/v1/tenants/{tenant_id}/quota
```

**响应：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "cpu": 100,
    "memory": 512,
    "storage": 10000,
    "vgpu": 0,
    "vram": 0,
    "gpu": 8
  }
}
```

#### 1.2 更新租户配额

```
PUT /api/v1/tenants/{tenant_id}/quota
```

**请求体：**
```json
{
  "cpu": 200,
  "memory": 1024,
  "storage": 20000,
  "vgpu": 0,
  "vram": 0,
  "gpu": 16
}
```

**约束：** 更新后所有工作空间配额之和 ≤ 新的租户配额

#### 1.3 获取租户已分配配额汇总

```
GET /api/v1/tenants/{tenant_id}/quota/summary
```

返回租户总配额、已分配给工作空间的配额、剩余可用配额。

**响应：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": { "cpu": 100, "memory": 512, ... },
    "allocated": { "cpu": 60, "memory": 300, ... },
    "available": { "cpu": 40, "memory": 212, ... }
  }
}
```

### 2. 工作空间级配额

#### 2.1 获取工作空间配额

```
GET /api/v1/workspaces/{workspace_id}/quota
```

**响应：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "cpu": 40,
    "memory": 200,
    "storage": 5000,
    "vgpu": 0,
    "vram": 0,
    "gpu": 4
  }
}
```

#### 2.2 更新工作空间配额

```
PUT /api/v1/workspaces/{workspace_id}/quota
```

**请求体：**
```json
{
  "cpu": 50,
  "memory": 250,
  "storage": 6000,
  "vgpu": 0,
  "vram": 0,
  "gpu": 4
}
```

**约束：** 更新后该工作空间配额 ≤ 所属租户的剩余可用配额（租户配额 - 其他工作空间配额之和）

#### 2.3 创建工作空间时初始化配额

```
POST /api/v1/workspaces
```

**请求体（扩展）：**
```json
{
  "name": "dev",
  "display_name": "开发环境",
  "quota": {
    "cpu": 40,
    "memory": 200,
    "storage": 5000,
    "vgpu": 0,
    "vram": 0,
    "gpu": 4
  }
}
```

**约束：** 新工作空间配额 ≤ 租户剩余可用配额。如果未传 `quota`，默认继承租户全量配额。

### 3. 创建租户时初始化配额

#### 3.1 创建租户（扩展）

```
POST /api/v1/tenants
```

**请求体（扩展）：**
```json
{
  "name": "dev",
  "display_name": "Dev Tenant",
  "quota": {
    "cpu": 100,
    "memory": 512,
    "storage": 10000,
    "vgpu": 0,
    "vram": 0,
    "gpu": 8
  }
}
```

**约束：** 如果未传 `quota`，默认分配 0（需平台管理员手动分配）。创建租户时自动创建 `default` 工作空间，其配额继承租户全量配额。

## 数据库迁移

### tenants 表

```sql
ALTER TABLE tenants ADD COLUMN quota JSONB NOT NULL DEFAULT '{}';
```

### workspaces 表

```sql
ALTER TABLE workspaces ADD COLUMN quota JSONB NOT NULL DEFAULT '{}';
```

## 前端对接

### 当前前端状态

| 页面 | 当前状态 | 对接后 |
|------|----------|--------|
| 创建租户弹窗 | 静态数据 | `POST /api/v1/tenants` 传 `quota` |
| 租户详情弹窗 | 静态数据 | `GET /api/v1/tenants/{id}/quota` 展示 |
| 工作空间配额编辑 | 本地 state | `PUT /api/v1/workspaces/{id}/quota` 持久化 |
| 租户列表 | 配额统计卡片 | `GET /api/v1/tenants/{id}/quota/summary` 展示 |

### 对接优先级

1. `GET /api/v1/tenants/{tenant_id}/quota` — 租户详情展示
2. `PUT /api/v1/tenants/{tenant_id}/quota` — 租户配额编辑
3. `GET /api/v1/workspaces/{workspace_id}/quota` — 工作空间配额展示
4. `PUT /api/v1/workspaces/{workspace_id}/quota` — 工作空间配额编辑
5. `GET /api/v1/tenants/{tenant_id}/quota/summary` — 租户配额汇总