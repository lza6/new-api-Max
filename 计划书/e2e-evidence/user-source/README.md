# 用户注册来源显示（User.Source）真实验证

- 密码注册新用户 `src_e2e_1` → 用户列表「注册方式」列显示「密码注册」（截图 users-source.png）
- 存量用户 root（无 source）→ 显示「—」
- 后端: model.User.Source 字段（varchar64, 自动迁移）；注册入口打标：
  - 密码/邮箱注册 → `password`
  - 管理员创建 → `admin`（后台建号 + setup root）
  - OAuth 内置 provider → `github / discord / wechat / telegram / linuxdo / oidc`
  - 自定义 OAuth provider → slug
- 单元测试：`TestOAuthSourceForProvider`（controller/oauth_source_test.go）
