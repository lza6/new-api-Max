package training_capture

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// OneDrive 后端的**读取**能力（SinkReader，可选接口）。
//
// 供管理端云盘浏览 / 文件预览 / 对话深度分析（Spec-008 增量 B4/B7/B9）。
// 写入与读取分离的原因见 sink.go 对 SinkReader 的说明。
//
// ## 实测确认的 Graph 行为（沿用上传侧已验证的坑）
//
// `GET /root:/path:/content` 返回 **302** 到 SharePoint 下载地址——
// 必须跟随重定向才能拿到内容（与上传侧「读回」是同一坑）。
// Go 的 http.Client 默认跟随重定向，但**不会把 Authorization 头
// 带到跨域的重定向目标**——这恰好是我们需要的：302 目标是预签名的
// SharePoint URL，自带凭据，再带 Graph token 反而多余（但无害）。

const (
	// onedriveReadMaxBytes 默认单文件读取上限（64MB）。
	// 分片是压缩的（实测压缩率 ~10%），64MB 压缩 ≈ 600MB 原始对话，
	// 远超实际单分片大小；防的是把未知巨大文件整个读进内存。
	onedriveReadMaxBytes = 64 << 20

	// onedriveTreeMaxEntries 目录树列举的条目硬顶。
	// 全遍历 5TB 云盘既慢又打爆 API 配额；管理端浏览有分页与目录下钻。
	onedriveTreeMaxEntries = 5000
)

// ReadFile 读取云盘文件（原始字节，不解释格式）。
func (s *oneDriveSink) ReadFile(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = onedriveReadMaxBytes
	}
	token, err := s.token(ctx)
	if err != nil {
		return nil, err
	}

	itemPath := s.itemPath(path)
	if itemPath == "" {
		return nil, fmt.Errorf("onedrive read: empty path")
	}
	endpoint := s.driveURL("/root:/" + itemPath + ":/content")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	// 复用上传侧的独立超时口径：大文件在慢链路下读取较久。
	readCtx, cancel := context.WithTimeout(ctx, onedriveUploadTimeout)
	defer cancel()

	resp, err := s.client.Do(req.WithContext(readCtx))
	if err != nil {
		return nil, fmt.Errorf("onedrive read %q failed: %w", itemPath, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("onedrive read %q: not found", itemPath)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("onedrive read %q returned %d", itemPath, resp.StatusCode)
	}

	// **上限 +1 探测**：读到 maxBytes+1 说明文件超限 → 报错而非截断
	// （半个文件会让分析端把残缺当完整，比「读不到」危险）。
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("onedrive read %q body: %w", itemPath, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrFileTooLarge
	}
	return data, nil
}

// DeleteFile 删除云盘文件。
//
// 语义：仅**文件**；对目录调用返回错误（防误删整棵子树——
// 目录删除必须走更谨慎的管理流程，不放在「删一个文件」的路径里）。
func (s *oneDriveSink) DeleteFile(ctx context.Context, path string) error {
	token, err := s.token(ctx)
	if err != nil {
		return err
	}

	itemPath := s.itemPath(path)
	if itemPath == "" {
		return fmt.Errorf("onedrive delete: empty path")
	}
	endpoint := s.driveURL("/root:/" + itemPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("onedrive delete %q failed: %w", itemPath, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	// 204 成功；404 视为已删除（幂等——重复点击不报错）。
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("onedrive delete %q returned %d", itemPath, resp.StatusCode)
}

// ListTree 递归列举目录树（BFS，带条目硬顶）。
//
// 沿用 CountItems 的防护思路：完整遍历 5TB 云盘会打爆 API 配额，
// 管理端浏览配合「展开下钻」逐层看，单次列举有硬顶（默认 5000 条）。
//
// ## 路径语义：**始终返回相对云盘根目录的完整路径**
//
// 无论 `root` 传什么，`TreeEntry.Path` 都是**可直接回传给
// `ReadFile` / `DeleteFile` / 再次 `ListTree` 的完整路径**。
//
// [修复防御] 早期实现把返回值裁成「相对 root」。看起来更"干净"，
// 但三个调用方都把它当**完整路径**用：
//   - 前端下钻：拿 `entry.path` 当下一层的 `path` 参数
//     → 少一层前缀 → Graph 404 → 被当成空目录 → 界面「没有文件」；
//   - 分析：`ListTree(会话目录)` 后直接 `ReadFile(e.Path)`
//     → 404 → 所有分片计入 SkippedParts → 报「没有内容」；
//   - 预览/删除：同一条路径语义。
//
// 也就是说「相对 root」这个契约没有任何调用方遵守，
// 而它导致的失败会伪装成「数据为空」，极难定位。
//
// 现在统一成完整路径：`root==""` 时行为与旧版一致，
// `root!=""` 时返回带前缀的完整路径，调用方无需任何拼接。
//
// [修复防御] 早期实现还把「待访问目录」只记叶子名，下钻第二层时
// 前缀就丢了（`2026-10-14/alice-7` 会退化成 `gpt-4o`），
// 导致后续目录全部列举失败、树是不完整的。队列里存的始终是完整路径。
// ListTree 列举目录树（丢弃截断标志的便捷包装）。
//
// 只用于**明确不在意截断**的调用方（如管理端浏览，它本来就要分页下钻）。
// 需要知道「是否拿到全部」的调用方（如**导出**）必须用
// `ListTreeWithTruncation` —— 把截断结果当全量打包会产出半包数据。
func (s *oneDriveSink) ListTree(ctx context.Context, root string, maxEntries int) ([]TreeEntry, error) {
	entries, _, err := s.ListTreeWithTruncation(ctx, root, maxEntries)
	return entries, err
}

// ListTreeWithTruncation 同 ListTree，但**如实报告是否被截断**。
//
// [修复防御] 盲点 B13：`ListTree` 内部会把条目夹到全局硬顶，
// 但调用方无从知道「我拿到的是全部还是前 N 项」。
//
// 对**导出**而言这是数据损坏级问题：导出按枚举结果打包，
// 截断时会产出一个**看似完整、实为前 5000 项**的归档，
// 而 manifest 里的 PartCount 还是那个偏小的数。
// 管理员会拿它去训练，以为拿到了全量数据。
//
// 因此这里把截断事实**显式返回**，让导出方能在截断时明确报错
// （而不是产出半包）。与 `List`/`CountItems` 的既有约定一致 ——
// 它们都已经用下界标志表达「这不是全部」。
func (s *oneDriveSink) ListTreeWithTruncation(
	ctx context.Context, root string, maxEntries int,
) ([]TreeEntry, bool, error) {
	if maxEntries <= 0 {
		maxEntries = onedriveTreeMaxEntries
	}
	if maxEntries > onedriveTreeMaxEntries {
		maxEntries = onedriveTreeMaxEntries // 请求再大也不越过全局硬顶
	}

	token, err := s.token(ctx)
	if err != nil {
		return nil, false, err
	}

	// 归一化 root（去掉首尾斜杠）后只用于**发起列举**，不参与返回值裁剪。
	// 返回值始终是完整路径，见本函数上方「路径语义」的说明。

	type queueItem struct {
		// fullPath 相对**云盘根**的完整路径（直接传给 fetchEntries）。
		fullPath string
		depth    int
	}
	queue := []queueItem{{fullPath: root, depth: 0}}
	out := make([]TreeEntry, 0, 64)
	// truncated: 是否因上限而**未列完**（可能是条目到顶，也可能是目录没走完）。
	truncated := false

	for len(queue) > 0 {
		if len(out) >= maxEntries {
			// 还有待访问目录 → 一定没列完。
			truncated = true
			break
		}
		item := queue[0]
		queue = queue[1:]

		entries, err := s.fetchEntriesWithMeta(ctx, token, item.fullPath)
		if err != nil {
			// 单目录失败不中断整体（部分结果比没有有用）——与 CountItems 一致。
			// **但必须标记截断**：少读了一个目录意味着结果不完整，
			// 不标记会让调用方（导出）把残缺当全量。
			truncated = true
			continue
		}
		for _, e := range entries {
			if len(out) >= maxEntries {
				truncated = true
				break
			}
			// 子项的**完整路径**：父路径 + 名字（父为空则是根下的直接子项）。
			full := e.name
			if strings.Trim(item.fullPath, "/") != "" {
				full = strings.Trim(item.fullPath, "/") + "/" + e.name
			}
			out = append(out, TreeEntry{
				Path:       full,
				IsDir:      e.isFolder,
				SizeBytes:  e.size,
				ModifiedAt: e.modified,
			})
			// 目录深度硬顶：防异常深/自引用的树把 BFS 拖爆。
			if e.isFolder && item.depth+1 < 6 {
				queue = append(queue, queueItem{fullPath: full, depth: item.depth + 1})
			} else if e.isFolder {
				// 到深度上限：明确告知有子项未展开，而不是静默当成空目录。
				out = append(out, TreeEntry{
					Path:  full + "/",
					IsDir: true,
				})
				// 深度到顶也是「没列完」—— 导出方需要知道。
				truncated = true
			}
		}
	}
	return out, truncated, nil
}

// entryMeta 带元数据的目录项（内部用）。
type entryMeta struct {
	name     string
	isFolder bool
	size     int64
	modified int64
}

// fetchEntriesWithMeta 列举目录（含 size/modified；与管理端展示对应）。
func (s *oneDriveSink) fetchEntriesWithMeta(ctx context.Context, token, path string) ([]entryMeta, error) {
	itemPath := s.itemPath(path)
	var endpoint string
	if itemPath == "" {
		endpoint = s.driveURL("/root/children?$top=" + fmt.Sprint(onedriveListPageLimit))
	} else {
		endpoint = s.driveURL("/root:/" + itemPath + ":/children?$top=" + fmt.Sprint(onedriveListPageLimit))
	}

	out := make([]entryMeta, 0, 32)
	next := endpoint
	for next != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return out, nil // 目录不存在 → 已收集的部分（通常为空）
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("onedrive list entries returned %d", resp.StatusCode)
		}

		var page struct {
			Value []struct {
				Name       string `json:"name"`
				Folder     *any   `json:"folder"`
				Size       int64  `json:"size"`
				ModifiedAt string `json:"lastModifiedDateTime"`
			} `json:"value"`
			NextLink string `json:"@odata.nextLink"`
		}
		if err := common.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		for _, it := range page.Value {
			e := entryMeta{name: it.Name, isFolder: it.Folder != nil, size: it.Size}
			if t, err := time.Parse(time.RFC3339, it.ModifiedAt); err == nil {
				e.modified = t.Unix()
			}
			out = append(out, e)
		}
		// 跟随 @odata.nextLink（它是绝对 URL，直接用）。
		if page.NextLink != "" {
			if _, err := url.Parse(page.NextLink); err != nil {
				return nil, fmt.Errorf("invalid nextLink")
			}
		}
		next = page.NextLink
	}
	return out, nil
}

// 编译期断言：oneDriveSink 实现了 SinkReader。
var _ SinkReader = (*oneDriveSink)(nil)
