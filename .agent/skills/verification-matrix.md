# Skill：验证矩阵（按风险选层，不机械全跑）

## 适用场景
任何声称「已完成 / 已修复 / 可上线」之前的自检。目标是**用最小代价拿到真证据**。

## 前置读取
- `test_ledger.md` 已知噪声表（哪些失败是既有的，别当回归）
- `.agent/memory/verified-scope.md`（哪些已验过，别重复）
- 本次改动的 diff（决定触达哪些接缝）

## 分层选择（按风险递增）

| 层 | 命令 | 何时必须跑 |
|---|---|---|
| L0 格式化 | `gofmt -l <files>` / `bun run format:check` | 每次 |
| L1 编译 | `go build ./...` | 每次 |
| L2 定向单测 | `go test -run "<Test>" ./<pkg>/` | 每次 |
| L3 包全量 | `go test ./<pkg>/` | 改动了该包的核心逻辑 |
| L4 类型/Lint | `bun run typecheck` / `bun run lint` | 前端改动 |
| L5 依赖包 | `go test ./service/ ./setting/...` | 改了被多方引用的基础设施 |
| L6 三库矩阵 | 见下 | 动 schema / 迁移 / 方言 SQL |
| L7 前端全量 | `bun run test:shards`（5 片，约 567s） | 收尾 |
| L8 生产冒烟 | 见下 | 部署后 |

**不要**为改一个按钮的间距跑 L7；也不要因为「只是改个日志」跳过 L6（那是方言坑的高发区）。

## 三库矩阵（动 DB 时强制）
```bash
# 至少覆盖：新建库 + 旧版本升级库，且启动两次证明迁移幂等
go test -run "Conformance|Matrix" ./model/ -count=1
```
- SQLite 用 `:memory:` 真库
- MySQL / PG 需真实实例（本机 PG 曾损坏，记临时实例解法于 `batch15` 记忆）
- **无实例时明确标注 `待验证`，不得声称「三库兼容」**

## 生产冒烟（部署后）
```bash
# 全部在服务器本机执行，不要用本机 curl
curl -s -D - -o /dev/null https://DOMAIN/api/status | grep -i x-new-api-version
for i in $(seq 1 20); do curl -s -o /dev/null -w "%{http_code}\n" https://DOMAIN/api/status; done | sort | uniq -c
docker logs new-api --since 10m 2>&1 | grep -cE "\| (500|502|503|504) \|"
docker exec postgres psql -U newapi -d new-api -X -c "select ..."   # 关键数据校验
```

## 验收标准
- 每层都必须有**实际输出**，不接受「应该没问题」
- 定向单测通过 ≠ 包全量通过（曾因只跑 `-run` 而漏掉 panic）
- mock 通过 ≠ 真实集成通过
- 无法实测的写清：做到哪一步、缺什么条件、如何降级

## 证据要求
`evidence/<task_id>.json`：
```json
{"task_id":"","status":"passed|partial|blocked","commands_executed":[{"cmd":"","exit_code":0,"raw_output_tail":"原文末尾"}],
 "files_changed":[],"tests":[],"seams_touched":[],"limitations":[]}
```
禁止：伪造退出码、把静态阅读写成运行验证、填写猜测的覆盖率。

## 常见坑
1. **「单独跑 PASS、全量 FAIL」必须先定性**：`git stash` 掉自己的改动再跑一遍，证明是否既有问题，然后记进 `test_ledger.md`
2. `go test ./controller/` 全量约 600s 会超时（jsplugin/sobek 栈），用 `-run` 缩小
3. 前端 `go:embed web/dist` 是**编译期快照**：只改前端不重新构建 = 白测
4. 生产端浏览器验证需 SSH 隧道（本机直连被 RST），见 `prod-ui-e2e-tunnel` 记忆
5. 改了 HTTP 状态码语义后，必须 `grep` 全仓断言旧状态码的测试
