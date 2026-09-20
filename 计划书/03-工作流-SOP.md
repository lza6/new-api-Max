# 03-工作流 SOP

每个任务从「头脑风暴」到「推 main」按以下顺序执行，禁止跳步。

1. **头脑风暴**：先写思考/方案要点，不直接改代码（对应仓库行事准则「先讨论好计划再执行」）。
2. **方案 md**：在 `计划书/` 下按主题新建方案文件，必须包含：目标、影响面、验收标准、三库影响、回滚路径。
3. **TDD**：先写失败测试（t=需求/契约），再实现到通过；复用公共 helpers（testify require/assert）。
4. **实现**：直接、可读；后端 JSON 走 `common.*`；可选字段用指针+omitempty；billing 走 quota_math 饱和；改动 relaykit 需 `GOWORK=off go build ./...`。
5. **三库验证**：SQLite + MySQL + PostgreSQL 真实实例；迁移/启动跑两次证明幂等；记录版本与命令。
6. **证据**：浏览器/线上/数据库证据入 `计划书/e2e-evidence/` 或 `audit-ledger.md`；静态/未跑/被阻分开标注。
7. **提交**：按主题拆 commit（如 `feat(x): ...` / `fix(x): ...` / `docs(plan): ...`），不 `git add .`。
8. **推送**：`git fetch origin main` 确认快进 → `git push origin main`，严禁 -f；远端 SHA 单独验证。