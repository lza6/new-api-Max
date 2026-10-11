# GEO 无 cloaking 版本 —— 最终结果（Batch-13 / GEO-safe）

结论：**complete**。
未恢复 `stash@{0}`（`geo-v2 WIP: 问题式小节+按路由JSON-LD（含 cloaking 风险，需重做）`）。
未改动 `web/`、`docs/`、`service/`、`router/`、`main.go`，未改动标识与版权头。

仓库根：`C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api`

---

## 1) 改了哪些 file:line

只改了 2 个文件：`controller/spa_seo.go`、`controller/spa_seo_test.go`。
（`controller/misc.go` **未改**：需要的路由级文案表本来就在 `spa_seo.go`，无必要改动。）

### `controller/spa_seo.go`

| 位置 | 改动 |
|---|---|
| `:183-186` | 新增 `spaPolicyNavLinks`：`/user-agreement` → `User Agreement` / 用户协议；`/privacy-policy` → `Privacy Policy` / 隐私政策 |
| `:193`、`:207-221` | `spaInternalLinksHTML` 在既有 `<nav aria-label="Public pages">` 内追加上述两条可跟随链接（当前页为其一时跳过，避免自链接） |
| `:367` | `BuildSPAIndexHTML` 把 JSON-LD 追加到内容块**之后**（即 `#spa-seo-content` 容器**外**） |
| `:524`、`:535` | `spaBuildContentBlock` 把既有 `Meta.Caveat` 从裸 `<p>` 提升为 `<h2>{限制标题}</h2><p>{Caveat 原文}</p>` |
| `:554` | 新增 `spaLimitationHeading`：zh →「限制与边界」；en → `Limitations` |
| `:592` | 新增 `spaRouteSchemaType`：`/` → `WebSite`；`/about` → `AboutPage`；其余 → `WebPage`（**刻意不加 `SearchAction`**） |
| `:609` | 新增 `spaJSONLDScript`：生成 `<script type="application/ld+json">`，用 `common.Marshal` 序列化；序列化失败即跳过该条（绝不把半截 JSON 投到页面上） |
| `:657` | 新增 `spaCustomHomeContentConfigured`：读 `common.OptionMap["HomePageContent"]`，判断本部署是否使用默认落地页 |
| `:671` / `:699` / `:727` | 新增 `spaFAQItemsEN` / `spaFAQItemsZH` / `spaFAQJSONLD`：首页 6 条问答。EN 逐字取自 `web/src/features/home/components/sections/faq.tsx` 的 `FAQ_ITEMS`；ZH 逐字取自 `web/src/i18n/locales/zh.json` 中同键的值 |

### `controller/spa_seo_test.go`

| 位置 | 改动 |
|---|---|
| `:140` | 更新既有 `TestSPASEOInjectsCrawlableContentBlock` 的 nav 链接计数断言：`len(seoPublicPaths)-1` → `len(seoPublicPaths)-1+len(spaPolicyNavLinks)` |
| `:414` / `:422` / `:440` / `:557` / `:697` | 新增 5 个测试辅助函数：抽内容块、解析 JSON-LD、隔离首页选项、取公开路由路径列表、读前端语言包 |
| `:456` | 新增 `TestSPASEOHomeInjectsFAQPage` |
| `:501` | 新增 `TestSPASEONonHomeRoutesHaveNoFAQPage` |
| `:518` | 新增 `TestSPASEOHomeFAQFollowsDefaultLandingPage` |
| `:532` | 新增 `TestSPASEOLimitationsSectionOnEveryPublicRoute` |
| `:566` | 新增 `TestSPASEORouteStructuredDataTypes` |
| `:601` | 新增 `TestSPASEONavLinksUserAgreementAndPrivacyPolicy` |
| `:623` | 新增 `TestSPASEOPolicyRoutesStayOutOfSitemap` |
| `:637` | 新增 `TestSPASEONoQuestionSectionsInInjectedBlock` |
| `:668` | 新增 `TestSPASEOHomeFAQMatchesFrontendSource` |

注入后的实际结构（人工核对过，用于确认「注入内容 = 页面上真实存在的内容」）：

- `/`：`<h1>` + 摘要 + `<h2>限制与边界</h2>` + Caveat 原文 + 7 条 `<nav>` 链接 + 上游文档外链；JSON-LD 两条：`WebSite` + `FAQPage`(6 问)
- `/about`：`<h1>` + 摘要 + `<h2>限制与边界</h2>` + Caveat 原文 + nav + 外链；JSON-LD 一条：`AboutPage`

---

## 2) 为什么这样设计不构成 cloaking

注入块对用户不可见（React 挂载后由移除脚本删除），因此它只允许承载两类内容：

1. **页面上确实可见、且逐字一致的文案**；
2. **页面自身的既有说明**（Caveat 原文）。

具体保证：

- **FAQPage 是本批最高危项，因此做了三重保证**：
  - 文案与首页 FAQ accordion 同源（`faq.tsx` 的 `FAQ_ITEMS` + `zh.json` 的译文）；
  - 条数固定为 6；
  - `TestSPASEOHomeFAQMatchesFrontendSource` **直接读前端源码**逐条核对（EN 对 `faq.tsx`、ZH 对 `zh.json`）。前端一改文案而 Go 侧没跟上，用例立刻 FAIL —— 杜绝静默漂移成「页面上并不存在的问答」。
- **额外门控**：若本部署配置了自定义首页，前端 `Home` 会提前返回、**根本不渲染 FAQ**，此时注入 `FAQPage` 就是在描述页面上不存在的内容 → `spaCustomHomeContentConfigured()` 为真则不注入。判据与前端同源（同一个 `HomePageContent` 选项）。
- **结构化数据只声明可证明的事实**：页面类型（`WebSite` / `AboutPage` / `WebPage`）、名称、地址、语言。**无** `aggregateRating`、**无** `review`、**无** `author` —— 我们既没有评分/评价数据，也不是模型作者。
- **「限制与边界」小节只搬运既有 Caveat 原文**，一个新的断言都没加；测试逐路由比对 `<p>` 内容恰等于该路由的 `Caveat` 原文。
- **刻意零「问句 `<h2>` + 回答 + 事实清单」** —— 那正是上一版被判不可上线的根因（用户看不到、爬虫看得到）。
- **JSON-LD 放在容器外**：它是 `<script>`、不渲染给用户；若放进 `#spa-seo-content` 内，会被移除脚本在 React 挂载后一并删掉，白白丢掉结构化数据。

---

## 3) TDD 红绿证据

### 绿（真实执行，均有输出）

| 命令 | 结果 |
|---|---|
| `gofmt -l controller/spa_seo.go controller/spa_seo_test.go controller/misc.go` | 无输出 |
| `go build ./...` | 通过（无输出） |
| `go vet ./controller/` | 通过（无输出） |
| `go test ./controller/ -run 'TestSPASEO\|TestGetSitemapXml\|TestGetRobotsTxt\|TestSPATrimFloat\|TestSPAPricing' -count=1 -v` | 26 个用例全 PASS，`ok github.com/lza6/new-api-Max/controller 18.508s` |

### 红（每条新断言做「改坏 → FAIL → 恢复 → PASS」）

11 处变异**全部被捕获**。变异脚本放在仓库外临时目录，跑完已还原（`git diff --stat` 只剩上述 2 个文件）。

| 变异 | 改坏内容 | 触发 FAIL 的用例 |
|---|---|---|
| M1 | 不注入 FAQPage | `TestSPASEOHomeInjectsFAQPage` |
| M2 | 所有路由都注入 FAQPage | `TestSPASEONonHomeRoutesHaveNoFAQPage` |
| M3 | 去掉自定义首页门控 | `TestSPASEOHomeFAQFollowsDefaultLandingPage` |
| M4 | 英文首条问句改一字 | `TestSPASEOHomeInjectsFAQPage` + `TestSPASEOHomeFAQMatchesFrontendSource` |
| M5 | 中文第 4 条答案改一字 | `TestSPASEOHomeFAQMatchesFrontendSource` |
| M6 | 去掉限制小节标题 | `TestSPASEOLimitationsSectionOnEveryPublicRoute` |
| M7 | 把 `<h2>` 改成问句 | `TestSPASEONoQuestionSectionsInInjectedBlock` |
| M8 | 首页类型改错（`WebPage`） | `TestSPASEORouteStructuredDataTypes` |
| M9 | 塞入 `aggregateRating` | `TestSPASEORouteStructuredDataTypes` |
| M10 | 删掉政策 nav 链接 | `TestSPASEONavLinksUserAgreementAndPrivacyPolicy` |
| M11 | 政策标签改成 `Terms of Service` | `TestSPASEONavLinksUserAgreementAndPrivacyPolicy` |

**诚实补充（一处过程发现）**：M8 第一轮**未被捕获**。首次变异只改了 `spaRouteSchemaType`，但首页分支当时**硬编码**了 `"WebSite"`，导致 `spaRouteSchemaType` 的 `case ""` 实际是死分支，改它不影响输出。据此把两个分支改为共用同一个 `schemaType` 局部变量（单一来源、消除重复字面量），重跑后 M8 转为 RED。

---

## 4) 刻意不做的 GEO 审计建议及原因

1. **问题式标题 / 直接回答 / 事实清单**（上一版 `stash@{0}` 的写法）
   这些内容在页面上并不存在 —— 用户看不到、爬虫看得到，属典型 cloaking，Google 会惩罚。审计原稿点名、而源码无法证明的三处断言（「每个请求都用 per-user API key 鉴权」「缓存命中与上下文长度阶梯会改变倍率」「客户端占比来自 User-Agent」）同属此类。一律不做，并用 `TestSPASEONoQuestionSectionsInInjectedBlock` 反向锁死。
2. **`aggregateRating` / `review` / `author`**
   没有评分、评价数据，也不是模型作者；声明它们等于向搜索引擎提供不实信息。
3. **`SearchAction` / 站点搜索**
   本站没有站点级搜索端点，声明它等于挂一个不存在的功能。
4. **不把 FAQ 渲染成可见 HTML 小节**
   本任务只要 JSON-LD；而往不可见块里塞问答，正是上一版被判死的模式。
5. **不把 `/user-agreement`、`/privacy-policy` 放进 `seoPublicPaths`**
   这两个前端路由确实存在，但正文来自后端配置项 `setting/system_setting/legal.go`、默认值是空字符串；纳入 `seoPublicPaths` 会让 `sitemap.xml` 收录两个可能为空的页面。改为只在站内导航里给出可跟随链接 —— 既不承诺页面有内容，又让爬虫能发现、用户能点。已用 `TestSPASEOPolicyRoutesStayOutOfSitemap` 锁住。
6. **未改 `controller/misc.go`**
   本次需要的路由级内容表都在 `spa_seo.go`，`misc.go` 无必要改动。

---

## 5) 未做项 / 残余（含一处需裁决的偏差）

- **需裁决的一处偏差**：任务给的文案是「服务条款」/`Terms of Service`，实现采用页面自身可见标题「用户协议」/`User Agreement`。依据：`web/src/features/legal/user-agreement.tsx` 标题为 `t('User Agreement')`（`zh.json` → 用户协议），`web/src/components/layout/components/footer.tsx` 的链接文案也是它；而「Terms of Service」在本仓库任何文案里都不存在。按「内容必须与用户看到的一致」原则取了前者。若要改回，只需改 `controller/spa_seo.go:184` 一行与对应测试期望。
- **未做真实浏览器 E2E**：`web/` 另有改动在进行，且本机 curl 的 HTTPS 全局不可用。本轮验证为 Go 级字符串/JSON 断言 + 11 组反向变异，**未**覆盖浏览器真实路径。
- **首页 JSON-LD 位于容器外**，因此 React 挂载后与前端运行时的 `landing-faq-jsonld` 并存（两者内容一致，按 Google 规则同一实体会被合并，不构成冲突）。若要彻底去重，可把 FAQPage 移进容器内，但那样就依赖前端 `useEffect` 必须补上，需要另行验证 —— 未擅自改动。
- **服务端仍只有 zh / en 两种语言**（沿用既有 `spaResolveLanguage`），fr / ja / ru / vi 访问者拿到的是中文整块内容 —— 既有行为，非本次引入。
- **未部署、未提交、未推送**。过程中 `service/site_visitor_tracker.go` 被他人改动导致过一次编译中断（与本改动无关，现已恢复）。
- 期间在 `web/e2e-local/` 下临时生成的核对脚本与 JSON 已删除，仓库内无因本任务新增的文件。
- **不作任何排名或流量提升的声明。**
