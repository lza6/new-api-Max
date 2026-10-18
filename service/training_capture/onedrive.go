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

// LoadOneDriveConfigFromEnv 从环境变量读取**主账号**配置。
// 必填项缺失时返回 `ErrSinkNotConfigured`（区别于「配了但连不上」）。
func LoadOneDriveConfigFromEnv() (OneDriveConfig, error) {
	return loadOneDriveConfigAt(1, "")
}

// maxOneDriveAccounts 最多支持几个账号（含主账号）。
//
// 取 9：用户当前 2 个；留出扩展空间但不至于让一个笔误的序号
// 悄悄变成"第 8 个账号"。
const maxOneDriveAccounts = 9

// oneDriveEnvNamesAt 返回第 idx 个账号（1 起）的环境变量名。
//
// 主账号沿用历史名字（`TRAINING_ONEDRIVE_TENANT`），
// 第 2 个起加序号前缀（`TRAINING_ONEDRIVE_2_TENANT`）—— 这样
// **老部署的配置一行都不用改**，加账号只需追加新的变量。
func oneDriveEnvNamesAt(idx int) (tenant, clientID, secret, driveID, rootPath string) {
	prefix := "TRAINING_ONEDRIVE_"
	if idx > 1 {
		prefix = fmt.Sprintf("TRAINING_ONEDRIVE_%d_", idx)
	}
	return prefix + "TENANT", prefix + "CLIENT_ID", prefix + "CLIENT_SECRET", prefix + "DRIVE_ID", prefix + "ROOT_PATH"
}

// loadOneDriveConfigAt 读取第 idx 个账号。
//
// defaultRootPath 用于第 2+ 个账号未显式配置 RootPath 时**沿用主账号**：
// 多账号应当共用同一套目录布局，否则管理端合并浏览会看到两套树。
func loadOneDriveConfigAt(idx int, defaultRootPath string) (OneDriveConfig, error) {
	tEnv, cEnv, sEnv, dEnv, rEnv := oneDriveEnvNamesAt(idx)
	root := strings.Trim(strings.TrimSpace(os.Getenv(rEnv)), "/")
	if root == "" {
		root = defaultRootPath
	}
	cfg := OneDriveConfig{
		Tenant:       strings.TrimSpace(os.Getenv(tEnv)),
		ClientID:     strings.TrimSpace(os.Getenv(cEnv)),
		ClientSecret: strings.TrimSpace(os.Getenv(sEnv)),
		DriveID:      strings.TrimSpace(os.Getenv(dEnv)),
		RootPath:     root,
	}
	var missing []string
	if cfg.Tenant == "" {
		missing = append(missing, tEnv)
	}
	if cfg.ClientID == "" {
		missing = append(missing, cEnv)
	}
	if cfg.ClientSecret == "" {
		missing = append(missing, sEnv)
	}
	if cfg.DriveID == "" {
		missing = append(missing, dEnv)
	}
	if len(missing) > 0 {
		// **不回显任何值**（secret 可能被打进日志）。
		return OneDriveConfig{}, fmt.Errorf("%w: missing %s", ErrSinkNotConfigured, strings.Join(missing, ", "))
	}
	return cfg, nil
}

// LoadOneDriveConfigsFromEnv 读取**全部**账号配置（主账号 + 可选的第 2..N 个）。
//
// 第 2+ 个账号的规则：
//   - **一个都没配**（连 TENANT 都空）→ 视为「没有这个账号」，跳过；
//   - **配了一部分**→ **明确报错**。半配的账号如果被静默跳过，管理员会以为
//     两个账号在负载均衡，实际只有一个在写 —— 与「B 也要用上」的诉求正好相反。
func LoadOneDriveConfigsFromEnv() ([]OneDriveConfig, error) {
	primary, err := LoadOneDriveConfigFromEnv()
	if err != nil {
		return nil, err
	}
	cfgs := []OneDriveConfig{primary}

	for idx := 2; idx <= maxOneDriveAccounts; idx++ {
		tEnv, cEnv, sEnv, dEnv, _ := oneDriveEnvNamesAt(idx)
		anySet := strings.TrimSpace(os.Getenv(tEnv)) != "" ||
			strings.TrimSpace(os.Getenv(cEnv)) != "" ||
			strings.TrimSpace(os.Getenv(sEnv)) != "" ||
			strings.TrimSpace(os.Getenv(dEnv)) != ""
		if !anySet {
			continue
		}
		c, cErr := loadOneDriveConfigAt(idx, primary.RootPath)
		if cErr != nil {
			return nil, fmt.Errorf("oneDrive account %d is partially configured: %w", idx, cErr)
		}
		cfgs = append(cfgs, c)
	}
	return cfgs, nil
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
	//
	// **多账号**：主账号用 `TRAINING_ONEDRIVE_*`，第 2+ 个用
	// `TRAINING_ONEDRIVE_<N>_*`。配了两个及以上时自动聚合成
	// `MultiAccountSink`（目录级粘性分片），worker 完全无感。
	RegisterSink("onedrive", func() (TrainingDataSink, error) {
		cfgs, err := LoadOneDriveConfigsFromEnv()
		if err != nil {
			return nil, err
		}
		sinks := make([]TrainingDataSink, 0, len(cfgs))
		for _, c := range cfgs {
			sinks = append(sinks, NewOneDriveSink(c))
		}
		if len(sinks) == 1 {
			return sinks[0], nil
		}
		m, mErr := NewMultiAccountSink(sinks)
		if mErr != nil {
			return nil, mErr
		}
		logMultiSinkStartup(m)
		return m, nil
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

// countTree 在给定目录树上统计目录数与文件数（**纯函数**，便于穷举边界）。
//
// 抽出来的理由：这个统计错了会让管理员看到**自信但错误**的数字
// （「N 个目录 / 0 个文件」且 truncated=false），而真机上复现它需要
// 真实的 OneDrive 目录层级。纯函数让「深度语义」可以离线钉死。
//
// 语义（与调用方约定）：
//   - 只**列出**深度 < maxDepth 的目录；被列出目录里的**非目录项**计入 files。
//   - 因此要数到第 N 层的文件，maxDepth 必须 ≥ N（目录自身在第 N 层时，
//     需要 maxDepth ≥ N+1 才会被展开）。
//   - 被计入 folders 但未展开的目录，其内部文件**不计入** —— 这是
//     「最多遍历 maxDepth 层」的必然结果，不是 bug；调用方要为此选对深度。
//   - visitedLimit > 0 时最多访问这么多个目录；**队列未排空**即表示数字是下界。
func countTree(list func(path string) ([]dirEntry, error), maxDepth, visitedLimit int) (folders, files int, truncated bool) {
	if maxDepth <= 0 {
		// 与 CountItems 的默认值保持一致：布局层数 + 1 才能数到
		// 模型目录**内部**的分片文件（见 CaptureLayoutDepth 的说明）。
		maxDepth = CaptureLayoutDepth + 1
	}
	type queueItem struct {
		path  string
		depth int
	}
	queue := []queueItem{{path: "", depth: 0}}
	visited := 0
	// readFailed 记录是否有目录**读取失败**。
	//
	// [修复防御] 曾经 `continue` 之后什么都不记，于是返回 `truncated=false`，
	// 管理员看到「N 个目录 / M 个文件」并当成权威规模 —— 而实际上
	// 有一整棵子树根本没被读到。这与「文件数恒为 0」是同一类错误：
	// **数字自信、且没有任何标志提示它不可信**。
	//
	// 读取失败与「达到访问上限」在语义上是同一件事：数字都是**下界**。
	// 因此并入 truncated 一并披露（宁可保守地说「至少」，不可假装完整）。
	readFailed := false

	for len(queue) > 0 && (visitedLimit <= 0 || visited < visitedLimit) {
		item := queue[0]
		queue = queue[1:]
		visited++

		entries, err := list(item.path)
		if err != nil {
			// 单个目录读失败不中断整体统计（部分结果比没有结果有用），
			// 但**必须**让调用方知道结果不完整。
			readFailed = true
			continue
		}
		for _, entry := range entries {
			if !entry.isFolder {
				files++
				continue
			}
			folders++
			if item.depth+1 < maxDepth {
				child := entry.name
				if item.path != "" {
					child = item.path + "/" + entry.name
				}
				queue = append(queue, queueItem{path: child, depth: item.depth + 1})
			}
		}
	}
	// 队列里还有待访问目录，或有目录读失败 → 数字是**下界**。
	return folders, files, len(queue) > 0 || readFailed
}

// CountItems 统计本程序根目录下的目录数与文件数（供管理员视图）。
//
// 只遍历到 maxDepth 层、且**最多访问 onedriveCountMaxDirs 个目录**
// ——完整遍历整个 5TB 云盘既慢又会打爆 API 配额，而管理员视图只需要「大概规模」。
//
// **调用方必须传 `CaptureLayoutDepth + 1`**：分片文件在**模型目录内部**，
// 传布局层数本身会让模型目录只被计数、不被展开 → files 恒为 0。
//
// [修复防御] 返回 truncated：达到上限就说明数字是**下界**而非总量。
// 不披露这一点会让管理员把 undercount 当成真实值去做扩缩容决策。
// 这与 `List` / `Stat` 的既有约定一致（未知用 0/标志表达，不假装是真实值）。
func (s *oneDriveSink) CountItems(ctx context.Context, maxDepth int) (folders, files int, truncated bool, err error) {
	if maxDepth <= 0 {
		// [修复防御] 默认值必须是**布局层数 + 1**，不是布局层数本身。
		//
		// 分片文件在模型目录**内部**；用布局层数作深度只会把模型目录计入
		// folders、却不展开它 → `files` 恒为 0，且 `truncated=false`
		// 让这个错误数字看起来完全可信。
		// 曾经调用处正是踩了这个坑；把错误答案留作默认值，会让**未来**
		// 的调用方再踩一次，所以默认值必须是正确答案。
		maxDepth = CaptureLayoutDepth + 1
	}
	// [修复防御] 队列起点必须是**空串**，不是 `s.cfg.RootPath`。
	//
	// `listEntries` 内部会经 `itemPath()` 再拼一次 RootPath。若这里也放
	// RootPath，实际请求变成 `/{Root}/.../{Root}/...` → Graph 404 →
	// `fetchEntries` 把 404 当「空目录」→ 本函数**静默返回 0/0/0**，
	// 而管理端会把这个 0 当权威值显示成「0 个目录 / 0 个文件」。
	// 目录层级由队列里的 path 逐级累积，与 RootPath 前缀无关。
	folders, files, truncated = countTree(func(path string) ([]dirEntry, error) {
		return s.listEntries(ctx, path)
	}, maxDepth, onedriveCountMaxDirs)
	// 单个目录读失败已在 countTree 内部跳过（部分结果比没有结果有用），
	// 并已并入 truncated 让调用方知道结果不完整；
	// 因此这里不产生 error —— 保持既有签名与「尽力统计」语义。
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
