//go:build linux

package manager

func EpollCreate() int {
	return syscall.EpollCreate1(0) // 仅 Linux 执行
}
