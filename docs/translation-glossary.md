# 翻译术语表 (Translation Glossary)

本文档为翻译贡献者提供项目中关键术语的标准翻译参考，以确保翻译的一致性和准确性。

This document provides standard translation references for key terminology in the project to ensure consistency and accuracy for translation contributors.

## 核心概念 (Core Concepts)

| 中文 | English | 说明 | Description |
|------|---------|------|-------------|
| 倍率 | Ratio | 用于计算价格的乘数因子 | Multiplier factor used for price calculation |
| 令牌 | Token | API访问凭证，也指模型处理的文本单元 | API access credentials or text units processed by models |
| 渠道 | Channel | API服务提供商的接入通道 | Access channel for API service providers |
| 分组 | Group | 用户或令牌的分类，影响价格倍率 | Classification of users or tokens, affecting price ratios |
| 额度 | Quota | 用户可用的服务额度 | Available service quota for users |

## 模型相关 (Model Related)

| 中文 | English | 说明 | Description |
|------|---------|------|-------------|
| 提示 | Prompt | 模型输入内容 | Model input content |
| 补全 | Completion | 模型输出内容 | Model output content |
| 输入 | Input/Prompt | 发送给模型的内容 | Content sent to the model |
| 输出 | Output/Completion | 模型返回的内容 | Content returned by the model |
| 模型倍率 | Model Ratio | 不同模型的计费倍率 | Billing ratio for different models |
| 补全倍率 | Completion Ratio | 输出内容的额外计费倍率 | Additional billing ratio for output content |
| 固定价格 | Price per call | 按次计费的价格 | Fixed price per call |
| 按量计费 | Pay-as-you-go | 根据使用量计费 | Billing based on usage |
| 按次计费 | Pay-per-view | 每次调用固定价格 | Fixed price per invocation |

## 用户管理 (User Management)

| 中文 | English | 说明 | Description |
|------|---------|------|-------------|
| 超级管理员 | Root User | 最高权限管理员 | Administrator with highest privileges |
| 管理员 | Admin User | 系统管理员 | System administrator |
| 普通用户 | Normal User | 普通权限用户 | Regular user with standard privileges |

## 充值与兑换 (Recharge & Redemption)

| 中文 | English | 说明 | Description |
|------|---------|------|-------------|
| 充值 | Top Up | 为账户增加额度 | Add quota to account |
| 兑换码 | Redemption Code | 可兑换额度的代码 | Code that can be redeemed for quota |

## 渠道管理 (Channel Management)

| 中文 | English | 说明 | Description |
|------|---------|------|-------------|
| 渠道 | Channel | API服务提供通道 | API service provider channel |
| 密钥 | Key | API访问密钥 | API access key |
| 优先级 | Priority | 渠道选择优先级 | Channel selection priority |
| 权重 | Weight | 负载均衡权重 | Load balancing weight |
| 代理 | Proxy | 代理服务器地址 | Proxy server address |
| 模型重定向 | Model Mapping | 请求体中模型名称替换 | Model name replacement in request body |

## 安全相关 (Security Related)

| 中文 | English | 说明 | Description |
|------|---------|------|-------------|
| 两步验证 | Two-Factor Authentication | 为账户提供额外安全保护的验证方式 | Additional security verification method for accounts |
| 2FA | Two-Factor Authentication | 两步验证的缩写 | Abbreviation for Two-Factor Authentication |

## 计费相关 (Billing Related)

| 中文 | English | 说明 | Description |
|------|---------|------|-------------|
| 倍率 | Ratio | 价格计算的乘数因子 | Multiplier factor used for price calculation |
| 倍率 | Multiplier | 价格计算的乘数因子（同义词） | Multiplier factor used for price calculation (synonym) |

## 翻译注意事项 (Translation Guidelines)

- **提示 (Prompt)** = 模型输入内容 / Model input content
- **补全 (Completion)** = 模型输出内容 / Model output content
- **倍率 (Ratio)** = 价格计算的乘数因子 / Multiplier factor for price calculation
- **额度 (Quota)** = 可用的用户服务额度，有时也翻译为 Credit / Available service quota for users, sometimes also translated as Credit
- **Token** = 根据上下文可能指 / Depending on context, may refer to:
  - API访问令牌 (API Token)
  - 模型处理的文本单元 (Text Token)
  - 系统访问令牌 (Access Token)

---

**贡献说明**: 如发现术语翻译不一致或有更好的翻译建议，欢迎提交 Issue 或 Pull Request。

**Contribution Note**: If you find any inconsistencies in terminology translations or have better translation suggestions, please feel free to submit an Issue or Pull Request.

## 限速与配额 (Rate Limit & Quota)

| 中文 | English | 说明 | Description |
|------|---------|------|-------------|
| 并发限制 | Concurrency Limit | 同时处理的请求数上限 | Max in-flight requests |
| 每分钟请求数 | RPM (Requests Per Minute) | 每分钟请求数上限 | Requests per minute limit |
| 配额 | Quota | 用户可用额度（计费单位） | User's billable balance |
| 额度用尽 | Insufficient Quota | 配额不足 | Balance exhausted |
| 分组 | Group | 渠道/用户的分组归属 | Group membership for channels/users |
| 分组倍率 | Group Ratio | 分组计费倍率 | Billing multiplier per group |
| 缓存倍率 | Cache Ratio | 缓存命中输入的计费倍率 | Billing multiplier for cached input |
| 影子价 | Shadow Price | 微美元等价费用（仅展示） | Micro-USD equivalent (display only) |
| 配额饱和 | Quota Saturation | 额度换算溢出被钳制（审计标记） | Clamped quota conversion (audit marker) |

## 任务插件 (Task Plugins)

| 中文 | English | 说明 | Description |
|------|---------|------|-------------|
| 任务插件 | Task Plugin | 异步任务（视频/图像）适配插件 | Async task adapter plugin |
| 审批闸门 | Execution Gate | 插件内容寻址审批 | Content-addressed plugin approval |
| 沙箱 | Sandbox | 插件执行受限环境 | Restricted plugin runtime |
| 轮询 | Polling | 异步任务状态轮询 | Async task status polling |
| 结构化进度 | Structured Progress | 任务进度分段数据 | Structured task progress |
| 回调 | Webhook | 任务状态变更回调 | Task state-change callback |
| 市场 | Marketplace | 插件市场 | Plugin marketplace |

## Web 防护 (Web Protection)

| 中文 | English | 说明 | Description |
|------|---------|------|-------------|
| Web 防护 | Web Protection | 管理端 Web 层防刷/限流/封禁 | Admin web-layer protection |
| 每秒请求数 | Limit Per Second | 每 IP 每秒请求上限 | Per-IP requests per second |
| 突发容忍 | Burst | 令牌桶突发容量 | Token-bucket burst capacity |
| 自动封禁 | Auto Ban | 触发阈值后自动封禁 IP | Auto-ban after threshold |
| 服务器状态 | Server Stats | 实时状态（网络/CPU/内存/磁盘） | Realtime server metrics |

## 订阅 (Subscription)

| 中文 | English | 说明 | Description |
|------|---------|------|-------------|
| 订阅档位 | Subscription Tier | 订阅套餐等级 | Subscription plan tier |
| 订阅限速 | Subscription Rate Limit | 订阅档位的并发/RPM 限制 | Subscription-tier rate limits |
| 需订阅分组 | Subscription Required Groups | 未订阅用户不可用的分组 | Groups requiring subscription |
