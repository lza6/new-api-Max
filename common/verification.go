package common

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type verificationValue struct {
	code string
	time time.Time
}

const (
	EmailVerificationPurpose = "v"
	PasswordResetPurpose     = "r"
)

var verificationMutex sync.Mutex
var verificationMap map[string]verificationValue
var verificationMapMaxSize = 10
var VerificationValidMinutes = 10

const verificationRedisPrefix = "verification:code:"
const verificationRedisLua = `
local v = redis.call('GET', KEYS[1])
if not v then return '' end
if v == ARGV[1] then
  redis.call('DEL', KEYS[1])
  return 'ok'
end
return 'mismatch'
`

// verificationRedisAvailable 返回是否可把验证码存到 Redis。
// G5：验证码从内存存储扩展为「Redis 优先、内存兜底」；Redis 不可用时
// 行为与旧版完全一致（内存 map + 互斥锁 + TTL 检查）。
func verificationRedisAvailable() bool {
	return RedisEnabled && RDB != nil
}

// verificationCodeTTL 返回 Redis 验证码过期时间。钳制最小 1 分钟：
// go-redis 中 expiration=0 表示「不设过期」，若有效期被误设为 0 会导致验证码永不过期。
func verificationCodeTTL() time.Duration {
	minutes := VerificationValidMinutes
	if minutes <= 0 {
		minutes = 1
	}
	return time.Duration(minutes) * time.Minute
}

func GenerateVerificationCode(length int) string {
	code := uuid.New().String()
	code = strings.Replace(code, "-", "", -1)
	if length == 0 {
		return code
	}
	return code[:length]
}

func RegisterVerificationCodeWithKey(key string, code string, purpose string) {
	rk := purpose + key
	if verificationRedisAvailable() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := RDB.Set(ctx, verificationRedisPrefix+rk, code, verificationCodeTTL()).Err(); err == nil {
			return
		} else {
			SysError("verification: redis set failed, falling back to memory: " + err.Error())
		}
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap[rk] = verificationValue{
		code: code,
		time: time.Now(),
	}
	if len(verificationMap) > verificationMapMaxSize {
		removeExpiredPairs()
	}
}

// redisVerificationGet 读取 Redis 中的验证码；返回 (code, found, err)。
func redisVerificationGet(rk string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	val, err := RDB.Get(ctx, verificationRedisPrefix+rk).Result()
	if err != nil {
		if err.Error() == "redis: nil" {
			return "", false, nil
		}
		return "", false, err
	}
	return val, true, nil
}

func VerifyCodeWithKey(key string, code string, purpose string) bool {
	rk := purpose + key
	if verificationRedisAvailable() {
		val, found, err := redisVerificationGet(rk)
		if err == nil && found {
			// Redis TTL 已实现过期；仅比对码值。
			return code == val
		}
		// Redis 未命中或故障：回读内存兜底（可能是故障窗口内写入内存的码）。
		// 注意：不在此处写回 Redis，避免与注册侧的单源策略产生不一致。
		if err != nil {
			SysError("verification: redis get failed, falling back to memory: " + err.Error())
		}
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	value, okay := verificationMap[rk]
	now := time.Now()
	if !okay || int(now.Sub(value.time).Seconds()) >= VerificationValidMinutes*60 {
		return false
	}
	return code == value.code
}

// VerifyCodeWithKeyConsume 校验验证码并原子消费（一次性）。
// T8：防止验证码在有效窗口内被重放用于多次注册/绑定。校验成功后立即删除，
// 与重置路径"成功后 DeleteKey"语义对齐。Redis 路径用 Lua 保证
// 「匹配才删除」的原子性，避免多实例并发消费竞态。
// Redis 未命中/故障时回读内存兜底；Redis 返回「码错」则不回退（防双重消费）。
func VerifyCodeWithKeyConsume(key string, code string, purpose string) bool {
	rk := purpose + key
	if verificationRedisAvailable() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		res, err := RDB.Eval(ctx, verificationRedisLua, []string{verificationRedisPrefix + rk}, code).Result()
		cancel()
		if err == nil {
			if s, ok := res.(string); ok {
				if s == "ok" {
					return true
				}
				if s == "mismatch" {
					// Redis 中码存在但值不匹配：不回退内存（避免对内存中的同键
					// 孤儿条目产生双重消费/误判）。
					return false
				}
			}
			// "" → Redis 未命中：回读内存兜底（故障窗口内写入的码）。
		} else {
			SysError("verification: redis consume failed, falling back to memory: " + err.Error())
		}
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	value, okay := verificationMap[rk]
	now := time.Now()
	if !okay || int(now.Sub(value.time).Seconds()) >= VerificationValidMinutes*60 {
		return false
	}
	if code != value.code {
		return false
	}
	delete(verificationMap, rk)
	return true
}

func DeleteKey(key string, purpose string) {
	rk := purpose + key
	if verificationRedisAvailable() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := RDB.Del(ctx, verificationRedisPrefix+rk).Err(); err != nil {
			SysError("verification: redis del failed: " + err.Error())
		}
		return
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	delete(verificationMap, rk)
}

// no lock inside, so the caller must lock the verificationMap before calling!
func removeExpiredPairs() {
	now := time.Now()
	for key := range verificationMap {
		if int(now.Sub(verificationMap[key].time).Seconds()) >= VerificationValidMinutes*60 {
			delete(verificationMap, key)
		}
	}
}

func init() {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap = make(map[string]verificationValue)
}
