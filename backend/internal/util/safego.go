// Package util 提供跨包的小工具。当前仅 safeGo：后台 goroutine 的统一防 panic 包装。
package util

import "log"

// SafeGo 启动一个后台 goroutine 并捕获 panic，避免任意后台任务 panic 导致整个进程崩溃。
// 网络中断、上游异常解析等导致的 panic 在此被兜底：打印堆栈式错误信息（含 name 标签），
// 绝不向上传播、绝不调用 os.Exit。供所有长生命周期后台任务（热加载/审计/配额/健康检查）
// 与短生命周期 fire-and-forget goroutine 统一使用。
func SafeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[safego] %s panic recovered: %v", name, r)
			}
		}()
		fn()
	}()
}
