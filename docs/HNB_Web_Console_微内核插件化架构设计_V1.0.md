# HNB Web Console 微内核插件化架构设计 V1.0

## 1. 设计目标

HNB Web Console 采用与后端一致的微内核架构：

-   Shell 微内核
-   Plugin 插件体系
-   Manifest 动态注册
-   按需加载
-   按需部署
-   模块化组装

目标是构建轻量、可扩展、企业级云原生控制台。

------------------------------------------------------------------------

## 2. 总体架构

    HNB Web Console

            Shell Kernel

     Auth | Layout | Context | Plugin Runtime

                  |

           Plugin Registry

                  |

     Dashboard
     Application
     Container
     Resource
     Service
     AI
     System

                  |

           HNB Backend API

                  |

     apiserver
     IAM
     Provider
     Extension
     Operation Worker

------------------------------------------------------------------------

## 3. Shell 微内核

Shell 只提供基础能力：

-   用户认证
-   页面布局
-   权限管理
-   上下文管理
-   插件加载
-   路由管理
-   全局事件
-   公共 UI 组件

目录：

    shell/

     core/
       auth
       permission
       context
       plugin-loader
       event-bus
       router

     layout/

     stores/

------------------------------------------------------------------------

## 4. 插件模型

插件包含：

    Plugin =
     UI Capability
     +
     Route
     +
     Menu
     +
     Permission
     +
     Backend Dependency

Manifest 示例：

``` json
{
 "name":"application",
 "displayName":"应用工厂",
 "enabled":true,
 "permissions":[
   "application:view"
 ],
 "dependencies":[
   "app-market",
   "kubernetes",
   "operation"
 ]
}
```

------------------------------------------------------------------------

## 5. 插件生命周期

    Install

    Register

    Enable

    Load

    Running

    Disable

    Upgrade

    Remove

------------------------------------------------------------------------

## 6. 微前端方案

采用 Hybrid Micro Frontend：

    Local Plugin
    +
    Remote Plugin

Local Plugin：

-   默认部署方式
-   适合私有化交付

Remote Plugin：

-   第三方扩展
-   AI生态
-   行业插件

------------------------------------------------------------------------

## 7. 功能插件划分

### Dashboard

负责：

-   平台总览
-   集群状态
-   资源状态
-   应用状态
-   Operation状态
-   告警

### Application Plugin

应用工厂：

    应用管理

    ├ 单体应用
    ├ 微服务应用
    ├ 环境管理
    ├ 应用模板
    ├ 应用市场
    ├ 发布
    ├ 灰度发布
    ├ 升级
    ├ 扩缩
    └ 应用监控

### Container Plugin

    容器

    ├ 集群实例
    ├ Workload
    ├ Pod
    ├ Namespace
    ├ Service
    ├ Config
    ├ Secret
    ├ Storage
    ├ Logs
    └ Events

### Resource Plugin

    资源

    ├ 集群管理
    ├ 节点管理
    ├ GPU资源
    ├ 网络
    ├ 存储
    ├ Agent
    └ GSLB

### Cloud Native Service Plugin

    云原生服务

    ├ 数据服务
    │  ├ MySQL
    │  ├ PostgreSQL
    │  └ Redis

    ├ 消息服务
    │  ├ Kafka
    │  └ RabbitMQ

    ├ 微服务治理

    └ 网关服务
       ├ API Gateway
       └ AI Gateway

### AI Plugin

    AI

    ├ 模型仓库
    ├ 推理服务
    ├ Agent
    └ 向量数据库

### System Plugin

    系统

    ├ 系统设置
    ├ 用户管理
    ├ 角色管理
    ├ 租户管理
    ├ 组织管理
    ├ 操作审批
    ├ 操作审计
    └ 扩展管理

------------------------------------------------------------------------

## 8. Context 上下文设计

不强制 Workspace。

采用：

``` typescript
interface HNBContext {

 tenantId?:string;

 spaceId?:string;

 environmentId?:string;

 clusterId?:string;

}
```

默认：

用户只选择空间。

复杂操作时选择：

-   租户
-   环境
-   集群

------------------------------------------------------------------------

## 9. 模块通信

插件禁止互相依赖。

采用：

    Shell Event Bus

    +

    Global Store

例如：

``` javascript
eventBus.emit(
"application.created"
)
```

------------------------------------------------------------------------

## 10. 权限模型

插件声明权限：

``` json
{
 "plugin":"resource",
 "permissions":[
   "cluster:view",
   "network:manage"
 ]
}
```

Shell 根据：

    User
    +
    Role
    +
    Tenant
    +
    Permission

动态生成菜单。

------------------------------------------------------------------------

## 11. 前后端扩展统一

形成：

    Backend Extension

    +

    Frontend Plugin

    +

    Manifest

例如：

安装 MySQL Extension：

后端：

    mysql-provider
    mysql-controller

前端：

    database-plugin

------------------------------------------------------------------------

## 12. 部署模式

### 最小部署

    Shell

    +

    Dashboard

### 标准部署

    Shell

    Dashboard

    Application

    Container

    Resource

    System

### 完整部署

增加：

    Service

    AI

    Security

------------------------------------------------------------------------

## 13. Helm 集成

``` yaml
frontend:

 enabled:true

 modules:

   dashboard:
     enabled:true

   application:
     enabled:true

   container:
     enabled:true

   resource:
     enabled:true

   ai:
     enabled:false
```

------------------------------------------------------------------------

## 14. 最终定位

HNB Web Console 不应只是 Kubernetes Dashboard。

而应定位为：

> 基于微内核和插件生态的企业级云原生应用管理控制台。

最终：

    HNB Shell

        |

    Plugin Runtime

        |

    Application
    Container
    Resource
    Service
    AI
    System

        |

    HNB Cloud Native Platform
