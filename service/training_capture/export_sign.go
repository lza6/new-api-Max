package training_capture

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// 导出「直链」的限时签名 token。
//
// ## 威胁模型
//
// 直链是**可分享的 URL**：一旦泄露，任何拿到它的人都能下载整个导出数据集
// （可能是几十 GB 的用户对话）。因此 token 必须：
//
//  1. **绑定导出 ID** —— 令牌只对某一个导出有效，不能拿 A 的令牌下 B 的数据
//     （防横向越权）。签名消息里含 exportID，改动即验签失败。
//  2. **限时** —— 内置过期时间戳并参与签名。过期后服务端拒绝，
//     不需要在服务端存任何状态（无状态校验，天然防「忘了清理」）。
//  3. **不可伪造** —— HMAC-SHA256，密钥是进程级的 `common.CryptoSecret`。
//     用 SHA-256 而非裸 base64(exportID)：后者任何人都能自造。
//  4. **常量时间比较** —— 用 `hmac.Equal`，防时序侧信道。
//
// ## 归属（谁下载的）
//
// 下载路由**不挂 AdminAuth**（否则 curl/aria2 无会话会被 401）。为了在无会话
// 时也能把「谁下载」记录到人，令牌把**签发者 ID（issuerID）**一并纳入签名，
// 并随 URL 的 `iss` 查询参数明文带出：改 `iss` 会让签名失配，无法嫁祸他人。
// 服务端据此把该次下载归到签发者名下（有会话时以会话为准）。
// **诚实边界**：若链接被转发，真正下载的人可能与签发者不同 —— 时间窗
// （见 exportAccessMaxTTL）是对这种风险的遏制，且下载记录里有 Range/来源/时间可审计。
//
// ## 为什么不防重放
//
// 这是**读**接口的下载令牌。读操作天然可重复：同一导出被同一个人多次下载
// （或续传时多次发 Range 请求）是**正常且必需**的 —— 一次性 token 会让
// 「断点续传」在第二个 Range 请求就 401。因此刻意**不做一次性消费**，
// 用「限时」遏制泄露窗口。

const (
	// exportAccessVersion 令牌格式版本（参与签名，便于将来演进）。
	exportAccessVersion = "ex1"

	// exportAccessDefaultTTL 直链默认有效期（1 小时）。
	// 太短会让大文件下载中途过期；太长会放大泄露窗口。
	exportAccessDefaultTTL = time.Hour

	// exportAccessMaxTTL 直链有效期上限（24 小时），夹取管理员请求的 TTL。
	exportAccessMaxTTL = 24 * time.Hour
)

// ErrExportAccessInvalid 直链 token 无效或过期。
var ErrExportAccessInvalid = errors.New("training export: access token invalid or expired")

// exportAccessMessage 构造签名消息：version \x00 exportID \x00 issuerID \x00 expiresAt。
//
// 各段都是整数，用 NUL 分隔是**结构性**防歧义（不依赖「整数不含分隔符」这一巧合）。
func exportAccessMessage(exportID uint, issuerID int, expiresAt int64) []byte {
	return []byte(exportAccessVersion + "\x00" +
		strconv.FormatUint(uint64(exportID), 10) + "\x00" +
		strconv.Itoa(issuerID) + "\x00" +
		strconv.FormatInt(expiresAt, 10))
}

// IssueExportAccess 签发一个绑定 (exportID, issuerID) 的限时直链 token。
//
// ttl<=0 用默认值；超过上限夹取到上限。
func IssueExportAccess(exportID uint, issuerID int, ttl time.Duration, now int64) (string, int64, error) {
	if exportID == 0 {
		return "", 0, ErrExportAccessInvalid
	}
	if common.CryptoSecret == "" {
		return "", 0, fmt.Errorf("training export: crypto secret not configured")
	}
	if ttl <= 0 {
		ttl = exportAccessDefaultTTL
	}
	if ttl > exportAccessMaxTTL {
		ttl = exportAccessMaxTTL
	}
	expiresAt := now + int64(ttl/time.Second)

	mac := hmac.New(sha256.New, []byte(common.CryptoSecret))
	_, _ = mac.Write(exportAccessMessage(exportID, issuerID, expiresAt))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	// token 形如 `<expiresAt>.<sig>`：过期时间**明文随行**，便于服务端
	// 在验签前先做快速过期判断（不必先算 HMAC）；它参与签名不可篡改。
	return strconv.FormatInt(expiresAt, 10) + "." + sig, expiresAt, nil
}

// VerifyExportAccess 校验 token 对 (exportID, issuerID) 是否有效且未过期。
//
// 校验顺序：分割 → 解析过期时间并做快速过期判断 → 常量时间验签。
// 「过期」与「伪造」返回同一结果（不区分），避免给攻击者信号。
func VerifyExportAccess(token string, exportID uint, issuerID int, now int64) bool {
	if exportID == 0 || common.CryptoSecret == "" {
		return false
	}
	expStr, sig, ok := strings.Cut(token, ".")
	if !ok || expStr == "" || sig == "" {
		return false
	}
	expiresAt, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || expiresAt <= now {
		return false
	}
	actualSig, err := base64.RawURLEncoding.Strict().DecodeString(sig)
	if err != nil || len(actualSig) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(common.CryptoSecret))
	_, _ = mac.Write(exportAccessMessage(exportID, issuerID, expiresAt))
	return hmac.Equal(actualSig, mac.Sum(nil))
}

// ClampExportTTL 把请求的 TTL（秒）夹取到 [1, maxTTL]。返回 0 表示用默认。
func ClampExportTTL(seconds int) time.Duration {
	if seconds <= 0 {
		return 0 // 交由 IssueExportAccess 用默认
	}
	d := time.Duration(seconds) * time.Second
	if d > exportAccessMaxTTL {
		return exportAccessMaxTTL
	}
	return d
}
