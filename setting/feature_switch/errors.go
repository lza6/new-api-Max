package feature_switch

import (
	"fmt"
	"strings"
)

// 本文件的错误类型是**可被管理端渲染**的：每种拒绝都带上足够上下文（哪个 key、
// 合法取值是什么、缺哪个前置），这样 UI 能直接给出可操作的提示，而不是一句
// "invalid request"。它们都是普通 error，用 errors.As 取具体类型。

// UnknownSwitchError 表示 key 不在注册表内（fail-closed，拒绝任何未知开关）。
type UnknownSwitchError struct{ Key string }

func (e *UnknownSwitchError) Error() string {
	return fmt.Sprintf("feature switch %q is not registered", e.Key)
}

// NotEditableError 表示该开关在管理端只读（需通过 env + 重启变更）。
type NotEditableError struct{ Key string }

func (e *NotEditableError) Error() string {
	return fmt.Sprintf("feature switch %q is not editable from the admin console", e.Key)
}

// InvalidValueError 表示取值不在该开关的合法集合内。
type InvalidValueError struct {
	Key     string
	Value   string
	Allowed []string
}

func (e *InvalidValueError) Error() string {
	return fmt.Sprintf("feature switch %q got value %q, allowed: %s",
		e.Key, e.Value, strings.Join(e.Allowed, "|"))
}

// DependencyError 表示开启被拒：某个前置开关尚未生效。
type DependencyError struct {
	Key     string
	Missing string
}

func (e *DependencyError) Error() string {
	return fmt.Sprintf("feature switch %q requires %q to be enabled first", e.Key, e.Missing)
}
