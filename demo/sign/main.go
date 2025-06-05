package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func listen(name string, sigChan chan os.Signal) {
	sig := <-sigChan
	fmt.Printf("协程 %s 收到信号: %v\n", name, sig)
}

func main() {
	// 协程1的信号 channel，带缓冲区大小为1
	sigChan1 := make(chan os.Signal, 1)
	signal.Notify(sigChan1, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// 协程2的信号 channel，带缓冲区大小为1
	sigChan2 := make(chan os.Signal, 1)
	signal.Notify(sigChan2, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// 分别在两个协程中等待信号
	go listen("协程1", sigChan1)
	go listen("协程2", sigChan2)

	fmt.Println("请按 Ctrl+C 发送 SIGINT 信号...")
	// 防止主函数退出
	time.Sleep(10 * time.Second)
	fmt.Println("程序结束")
}
