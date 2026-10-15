package training_capture

import (
	"github.com/shirou/gopsutil/mem"
)

// readHostMemoryTotal 读宿主机总内存。
// 单独成文件：让 gopsutil 依赖只出现在这一处，且测试可以整体替换。
func readHostMemoryTotal() int64 {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return 0
	}
	return int64(vm.Total)
}
