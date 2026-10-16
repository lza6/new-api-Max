package training_capture

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// OneDrive（Microsoft Graph）后端。
//
// ## 认证方式（实测确认，见 HANDOFF_onedrive.md）
//
// 用 **client_credentials**（应用凭据）——**不需要浏览器登录、不需要用户密码**。
// 凭据是 Azure AD 应用的 client_id / client_secret + tenant + drive_id。
//
// ## 实测验证过的能力（2026-10-14，生产凭据）
//
//	token ✅  建目录 ✅  上传 ✅  读回逐字一致 ✅  列举 ✅  删除 ✅
//	配额：5TB 总 / 5.49TB 剩余
//
// ## 实测踩到的坑（都已在实现里处理）
//
//  1. **读回必须跟随重定向**：`.../content` 返回 302 到 SharePoint 下载地址，
//     不跟随会拿到空响应体（表现像「上传成功但读不到」）。
//  2. `conflictBehavior=replace` 让**建目录幂等**：并发/重复创建不报错。
//     实测 DELETE 返回 204，无 body。
//  3. 简单上传上限 4MB（Graph 的限制）。超过需 createUploadSession，
//     本实现把分片上限设在 4MB 以下，避免走复杂路径。

const (
	// graphBaseURL Microsoft Graph 端点。
	graphBaseURL = "https://graph.microsoft.com/v1.0"

	// onedriveTokenSkew token 提前续期窗口。
	//
	// 实测 token 有效期约 1 小时。提前 5 分钟续期，避免「刚好在过期瞬间发请求」
	// 导致一次失败——这类边界失败最难排查（偶发、无法稳定复现）。
	onedriveTokenSkew = 5 * time.Minute

	// onedriveRequestTimeout 单次 Graph API 请求超时。
	onedriveRequestTimeout = 30 * time.Second

	// onedriveUploadTimeout 上传超时（比普通请求长：分片可到 MB 级，上行仅 75Mbps）。
	onedriveUploadTimeout = 120 * time.Second

	// OneDriveSimpleUploadMaxBytes Graph 简单上传的硬上限（4MB）。
	// 留安全余量：超限会返回 413，走上传会话是另一套复杂逻辑。
	OneDriveSimpleUploadMaxBytes = 4 << 20

	// onedriveListPageLimit 列举时的分页大小。
	onedriveListPageLimit = 200

	// onedriveCountMaxDirs 管理员统计时最多访问的目录数（防打爆 API 配额）。
	onedriveCountMaxDirs = 500

	// onedriveListMaxEntries 单目录列举的条目上限。
	//
	// 超限时 `List` **报错而非静默截断**：截断的列表会让分片序号算小，
	// 进而覆盖已上传分片（不可逆）。取 5 万：按 2MB/分片 ≈ 100GB/天/用户/模型，
	// 远超实际；正常目录只有几十个分片。
	onedriveListMaxEntries = 50_000
)

// OneDriveConfig OneDrive 后端配置。
//
// **全部来自环境变量**（见 plan ADR-008-6）：明文密钥落库会让
// 「DB 备份泄漏 = 云盘失守」。
type OneDriveConfig struct {
	Tenant       string
	ClientID     string
	ClientSecret string
	DriveID      string
	// RootPath 云盘内的根目录（可选）。所有写入都在它下面，
	// 便于把本程序的数据与其它内容隔离。
	RootPath string
}

// Env names。集中定义便于文档与部署脚本引用。
const (
	EnvOneDriveTenant       = "TRAINING_ONEDRIVE_TENANT"
	EnvOneDriveClientID     = "TRAINING_ONEDRIVE_CLIENT_ID"
	EnvOneDriveClientSecret = "TRAINING_ONEDRIVE_CLIENT_SECRET"
	EnvOneDriveDriveID      = "TRAINING_ONEDRIVE_DRIVE_ID"
	EnvOneDriveRootPath     = "TRAINING_ONEDRIVE_ROOT_PATH"
)

// LoadOneDriveConfigFromEnv 从环境变量读取配置。
// 必填项缺失时返回 `ErrSinkNotConfigured`（区别于「配了但连不上」）。
func LoadOneDriveConfigFromEnv() (OneDriveConfig, error) {
	cfg := OneDriveConfig{
		Tenant:       strings.TrimSpace(os.Getenv(EnvOneDriveTenant)),
		ClientID:     strings.TrimSpace(os.Getenv(EnvOneDriveClientID)),
		ClientSecret: strings.TrimSpace(os.Getenv(EnvOneDriveClientSecret)),
		DriveID:      strings.TrimSpace(os.Getenv(EnvOneDriveDriveID)),
		RootPath:     strings.Trim(strings.TrimSpace(os.Getenv(EnvOneDriveRootPath)), "/"),
	}
	var missing []string
	if cfg.Tenant == "" {
		missing = append(missing, EnvOneDriveTenant)
	}
	if cfg.ClientID == "" {
		missing = append(missing, EnvOneDriveClientID)
	}
	if cfg.ClientSecret == "" {
		missing = append(missing, EnvOneDriveClientSecret)
	}
	if cfg.DriveID == "" {
		missing = append(missing, EnvOneDriveDriveID)
	}
	if len(missing) > 0 {
		// **不回显任何值**（secret 可能被打进日志）。
		return OneDriveConfig{}, fmt.Errorf("%w: missing %s", ErrSinkNotConfigured, strings.Join(missing, ", "))
	}
	return cfg, nil
}

// oneDriveSink OneDrive 后端实现。
type oneDriveSink struct {
	cfg    OneDriveConfig
	client *http.Client

	tokenMu      sync.Mutex
	accessToken  string
	tokenExpires time.Time
}

func init() {
	// 注册后端。用注册表让采集核心完全不知道 OneDrive 的存在。
	RegisterSink("onedrive", func() (TrainingDataSink, error) {
		cfg, err := LoadOneDriveConfigFromEnv()
		if err != nil {
			return nil, err
		}
		return NewOneDriveSink(cfg), nil
	})
}

// NewOneDriveSink 用给定配置构造后端。
func NewOneDriveSink(cfg OneDriveConfig) *oneDriveSink {
	return &oneDriveSink{
		cfg: cfg,
		// 复用项目的共享出站 client 约定（连接池复用）。
		// 但**不复用 SSRF-protected client**：Graph 是固定受信端点，
		// 且 SSRF 校验会拒绝我们主动配置的域名解析结果。
		client: &http.Client{Timeout: onedriveRequestTimeout},
	}
}

func (s *oneDriveSink) Name() string { return "onedrive" }

// MaxWritePartBytes 声明 Graph 简单上传的硬上限（4MB）。
//
// worker 据此切分，不再猜后端类型 —— 这正是 `PartUploadLimiter` 存在的理由。
func (s *oneDriveSink) MaxWritePartBytes() int { return OneDriveSimpleUploadMaxBytes }

// ---------- token ----------

// token 返回有效的 access token（带缓存与提前续期）。
func (s *oneDriveSink) token(ctx context.Context) (string, error) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()

	if s.accessToken != "" && time.Now().Before(s.tokenExpires.Add(-onedriveTokenSkew)) {
		return s.accessToken, nil
	}

	form := url.Values{}
	form.Set("client_id", s.cfg.ClientID)
	form.Set("client_secret", s.cfg.ClientSecret)
	form.Set("scope", "https://graph.microsoft.com/.default")
	form.Set("grant_type", "client_credentials")

	endpoint := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token",
		url.PathEscape(s.cfg.Tenant))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("onedrive token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		// **不回显 body 全文**：错误响应里可能带 client_secret 的片段。
		return "", fmt.Errorf("onedrive token request returned %d", resp.StatusCode)
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := common.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("onedrive token response parse failed: %w", err)
	}
	if payload.AccessToken == "" {
		return "", errors.New("onedrive token response contained no access_token")
	}

	s.accessToken = payload.AccessToken
	expiresIn := payload.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	s.tokenExpires = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return s.accessToken, nil
}

// itemPath 把相对路径拼成 Graph 的 item 路径（含可选 RootPath 前缀）。
//
// 路径里的每段都应已由 `SanitizeDirName` / `SanitizeModelName` 净化过；
// 这里再做一次 `url.PathEscape` 是**纵深防御**（万一调用方漏了净化）。
func (s *oneDriveSink) itemPath(path string) string {
	segments := make([]string, 0, 4)
	if s.cfg.RootPath != "" {
		segments = append(segments, strings.Split(s.cfg.RootPath, "/")...)
	}
	for _, segment := range strings.Split(path, "/") {
		segment = strings.TrimSpace(segment)
		if segment == "" || segment == "." || segment == ".." {
			continue
		}
		segments = append(segments, url.PathEscape(segment))
	}
	return strings.Join(segments, "/")
}

func (s *oneDriveSink) driveURL(suffix string) string {
	return fmt.Sprintf("%s/drives/%s%s", graphBaseURL, url.PathEscape(s.cfg.DriveID), suffix)
}

// EnsureDir 幂等创建目录（逐级）。
//
// 逐级创建而非一次创建整条路径：Graph 的 children 接口一次只建一级，
// 且**父目录必须已存在**。逐级 + conflictBehavior=replace 让重复调用安全。
func (s *oneDriveSink) EnsureDir(ctx context.Context, path string) error {
	segments := make([]string, 0, 4)
	if s.cfg.RootPath != "" {
		segments = append(segments, strings.Split(s.cfg.RootPath, "/")...)
	}
	for _, segment := range strings.Split(path, "/") {
		segment = strings.TrimSpace(segment)
		if segment == "" || segment == "." || segment == ".." {
			continue
		}
		segments = append(segments, segment)
	}
	if len(segments) == 0 {
		return nil
	}

	current := ""
	for _, segment := range segments {
		if current == "" {
			if err := s.createFolder(ctx, "", segment); err != nil {
				return err
			}
			current = url.PathEscape(segment)
			continue
		}
		if err := s.createFolder(ctx, current, segment); err != nil {
			return err
		}
		current = current + "/" + url.PathEscape(segment)
	}
	return nil
}

// createFolder 在 parentPath（已转义）下创建名为 name 的目录。
// parentPath 为空表示根目录。
func (s *oneDriveSink) createFolder(ctx context.Context, parentPath, name string) error {
	token, err := s.token(ctx)
	if err != nil {
		return err
	}

	endpoint := s.driveURL("/root/children")
	if parentPath != "" {
		endpoint = s.driveURL("/root:/" + parentPath + ":/children")
	}

	payload, err := common.Marshal(map[string]any{
		"name":   name,
		"folder": map[string]any{},
		// 幂等关键：已存在时不报错、不抛 conflict。
		// **不用 `fail`**（会 409）也不用 `rename`（会产生 user-3 1 这样的垃圾目录）。
		"@microsoft.graph.conflictBehavior": "replace",
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("onedrive create folder %q failed: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	// 201 新建 / 200 已存在（`replace` 语义）。
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		return nil
	}
	// [修复防御] 409 有**两个**来源：(a) 并发下同名目录已存在（无害）；
	// (b) 该位置已存在一个**同名文件**。后者若被当成「目录已就绪」，
	// 后续走 `/root:/…/X:/children` 会全部失败，而错误会在**离根因很远的地方**
	// 才浮现（上游只报「children 不可用」），极难排查。
	// 因此 409 时**回读该路径确认它确实是目录**。
	if resp.StatusCode == http.StatusConflict {
		isFolder, verifyErr := s.isFolderAt(ctx, token, parentPath, name)
		if verifyErr != nil {
			return fmt.Errorf("onedrive create folder %q conflicted and verification failed: %w", name, verifyErr)
		}
		if !isFolder {
			return fmt.Errorf("onedrive create folder %q: a file with the same name already exists at that path", name)
		}
		return nil
	}
	return fmt.Errorf("onedrive create folder %q returned %d", name, resp.StatusCode)
}

// isFolderAt 回读 parentPath/name，报告它是否**确实是目录**。
func (s *oneDriveSink) isFolderAt(ctx context.Context, token, parentPath, name string) (bool, error) {
	full := name
	if parentPath != "" {
		full = parentPath + "/" + url.PathEscape(name)
	}
	endpoint := s.driveURL("/root:/" + full)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.client.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("stat %q returned %d", full, resp.StatusCode)
	}
	var item struct {
		Folder *any `json:"folder"`
		File   *any `json:"file"`
	}
	if err := common.Unmarshal(body, &item); err != nil {
		return false, err
	}
	if item.File != nil && item.Folder == nil {
		return false, nil
	}
	return item.Folder != nil, nil
}

// WritePart 上传分片。
//
// 走 Graph 的简单上传（`PUT .../content`），要求 data <= 4MB。
// 超过时**明确报错**而不是截断——截断会让云盘上的数据不完整且难以察觉。
func (s *oneDriveSink) WritePart(ctx context.Context, path string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("onedrive upload: empty payload")
	}
	if len(data) > OneDriveSimpleUploadMaxBytes {
		return "", fmt.Errorf("onedrive upload: %d bytes exceeds simple-upload limit %d; split the part first",
			len(data), OneDriveSimpleUploadMaxBytes)
	}

	token, err := s.token(ctx)
	if err != nil {
		return "", err
	}

	itemPath := s.itemPath(path)
	endpoint := s.driveURL("/root:/" + itemPath + ":/content")

	// 上传超时单独设：分片可达 MB 级而本站上行仅 75Mbps。
	uploadCtx, cancel := context.WithTimeout(ctx, onedriveUploadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(uploadCtx, http.MethodPut, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("onedrive upload %q failed: %w", itemPath, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("onedrive upload %q returned %d", itemPath, resp.StatusCode)
	}

	var result struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		WebURL string `json:"webUrl"`
		Parent any    `json:"parentReference"`
	}
	if err := common.Unmarshal(body, &result); err != nil {
		// 上传已成功（状态码 2xx）；解析失败不该让调用方以为失败并重传。
		return itemPath, nil
	}
	// **校验远端尺寸与本地一致**：不一致说明传输被截断，
	// 静默成功会在云盘留下不完整分片（最难发现的一类数据问题）。
	if result.Size != 0 && result.Size != int64(len(data)) {
		return "", fmt.Errorf("onedrive upload %q: remote size %d != local %d",
			itemPath, result.Size, len(data))
	}
	if result.WebURL != "" {
		return result.WebURL, nil
	}
	return itemPath, nil
}

// List 列举目录下的文件名（用于恢复分片序号）。
// 目录不存在时返回空切片——「还没写过」是正常状态，不是故障。
func (s *oneDriveSink) List(ctx context.Context, path string) ([]string, error) {
	token, err := s.token(ctx)
	if err != nil {
		return nil, err
	}

	itemPath := s.itemPath(path)
	if itemPath == "" {
		return nil, nil
	}
	endpoint := s.driveURL("/root:/" + itemPath + ":/children")

	names := make([]string, 0, 16)
	next := endpoint
	for next != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := s.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("onedrive list %q failed: %w", itemPath, err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()

		// 目录不存在 → 空结果（不是错误）。
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("onedrive list %q returned %d", itemPath, resp.StatusCode)
		}

		var page struct {
			Value []struct {
				Name   string `json:"name"`
				Folder *any   `json:"folder"`
			} `json:"value"`
			NextLink string `json:"@odata.nextLink"`
		}
		if err := common.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("onedrive list %q parse failed: %w", itemPath, err)
		}
		for _, item := range page.Value {
			// 只要文件（分片）；子目录忽略。
			if item.Folder == nil {
				names = append(names, item.Name)
			}
		}
		// [修复防御] 分页上限**不能静默截断**：`List` 的唯一用途是让
		// `PickNextPartSeq` 算出下一个分片序号。截断后若高序号在后面的分页里，
		// 算出的序号会**偏小** → **覆盖已上传的分片**（不可逆的数据破坏）。
		// 因此超限时**明确报错**，让调用方放弃本次上传（下轮重试），
		// 而不是带着一个错误的序号去做破坏性决策。
		if len(names) > onedriveListMaxEntries {
			return nil, fmt.Errorf(
				"onedrive list %q exceeded %d entries; refusing to return a truncated list because the caller uses it to pick the next part number",
				itemPath, onedriveListMaxEntries)
		}
		next = page.NextLink
	}
	return names, nil
}

// Stat 返回云盘配额。
func (s *oneDriveSink) Stat(ctx context.Context) (SinkStat, error) {
	token, err := s.token(ctx)
	if err != nil {
		return SinkStat{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.driveURL(""), nil)
	if err != nil {
		return SinkStat{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.client.Do(req)
	if err != nil {
		return SinkStat{}, fmt.Errorf("onedrive stat failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return SinkStat{}, fmt.Errorf("onedrive stat returned %d", resp.StatusCode)
	}

	var payload struct {
		DriveType string `json:"driveType"`
		Quota     struct {
			Total     int64  `json:"total"`
			Used      int64  `json:"used"`
			Remaining int64  `json:"remaining"`
			State     string `json:"state"`
		} `json:"quota"`
	}
	if err := common.Unmarshal(body, &payload); err != nil {
		return SinkStat{}, fmt.Errorf("onedrive stat parse failed: %w", err)
	}

	stat := SinkStat{
		TotalBytes:     payload.Quota.Total,
		UsedBytes:      payload.Quota.Used,
		RemainingBytes: payload.Quota.Remaining,
		Provider:       "onedrive",
	}
	// 文件夹/文件数不在 quota 里，由上层按需另查（避免每次 Stat 都遍历全盘）。
	return stat, nil
}

// CountItems 统计本程序根目录下的目录数与文件数（供管理员视图）。
//
// 只遍历到 maxDepth 层、且**最多访问 onedriveCountMaxDirs 个目录**
// ——完整遍历整个 5TB 云盘既慢又会打爆 API 配额，而管理员视图只需要「大概规模」。
//
// [修复防御] 返回 truncated：达到上限就说明数字是**下界**而非总量。
// 不披露这一点会让管理员把 undercount 当成真实值去做扩缩容决策。
// 这与 `List` / `Stat` 的既有约定一致（未知用 0/标志表达，不假装是真实值）。
func (s *oneDriveSink) CountItems(ctx context.Context, maxDepth int) (folders, files int, truncated bool, err error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	// 注意：这里**不再**取 s.cfg.RootPath —— 实际请求路径由 itemPath()
	// 统一拼 RootPath | 目录层级由 queueItem.path 逐级累积。
	// （原先的 `if root == "" { root = "" }` 是空操作，已删。）

	type queueItem struct {
		path  string
		depth int
	}
	// [修复防御] 队列起点必须是**空串**，不是 `s.cfg.RootPath`。
	//
	// `listEntries` 内部会经 `itemPath()` 再拼一次 RootPath。若这里也放
	// RootPath，实际请求变成 `/{Root}/.../{Root}/...` → Graph 404 →
	// `fetchEntries` 把 404 当「空目录」→ 本函数**静默返回 0/0/0**，
	// 而管理端会把这个 0 当权威值显示成「0 个目录 / 0 个文件」。
	// 目录层级由 queueItem.path 逐级累积，与 RootPath 前缀无关。
	queue := []queueItem{{path: "", depth: 0}}
	visited := 0

	for len(queue) > 0 && visited < onedriveCountMaxDirs {
		item := queue[0]
		queue = queue[1:]
		visited++

		entries, err := s.listEntries(ctx, item.path)
		if err != nil {
			// 单个目录读失败不中断整体统计（部分结果比没有结果有用）。
			continue
		}
		for _, entry := range entries {
			if entry.isFolder {
				folders++
				if item.depth+1 < maxDepth {
					child := entry.name
					if item.path != "" {
						child = item.path + "/" + entry.name
					}
					queue = append(queue, queueItem{path: child, depth: item.depth + 1})
				}
			} else {
				files++
			}
		}
	}
	// 队列里还有待访问目录 → 达到了访问上限，数字是下界。
	truncated = len(queue) > 0
	return folders, files, truncated, nil
}

type dirEntry struct {
	name     string
	isFolder bool
}

func (s *oneDriveSink) listEntries(ctx context.Context, path string) ([]dirEntry, error) {
	token, err := s.token(ctx)
	if err != nil {
		return nil, err
	}
	itemPath := s.itemPath(path)
	if itemPath == "" {
		// 根目录：用 /root/children（`/root::/children` 不合法）。
		endpoint := s.driveURL("/root/children?$top=" + fmt.Sprint(onedriveListPageLimit))
		return s.fetchEntries(ctx, token, endpoint)
	}
	endpoint := s.driveURL("/root:/" + itemPath + ":/children?$top=" + fmt.Sprint(onedriveListPageLimit))
	return s.fetchEntries(ctx, token, endpoint)
}

func (s *oneDriveSink) fetchEntries(ctx context.Context, token, endpoint string) ([]dirEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("onedrive list entries returned %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	var page struct {
		Value []struct {
			Name   string `json:"name"`
			Folder *any   `json:"folder"`
		} `json:"value"`
	}
	if err := common.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	out := make([]dirEntry, 0, len(page.Value))
	for _, item := range page.Value {
		out = append(out, dirEntry{name: item.Name, isFolder: item.Folder != nil})
	}
	return out, nil
}
