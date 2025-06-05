//go:build linux

package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"golang.org/x/sys/unix"
	"log"
	"net"
	"net/http"
	"reflect"
	"sync"
	"syscall"
	"time"
)

func main() {
	// 创建epoll
	epollFd, err := unix.EpollCreate(1)
	if err != nil {
		log.Fatalf("unable to create vepoll:%v\n", err)
	}

	connections := make(map[int32]*websocket.Conn, 1000000)
	var mu sync.Mutex
	ch := make(chan int32, 50)

	// 开启协程，处理接受到的信息
	go func() {
		for {
			select {
			case fd := <-ch:
				conn := connections[fd]
				if conn == nil {
					continue
				}
				// 接收消息
				_, message, err := conn.ReadMessage()
				fmt.Println("message:", string(message))
				time.Sleep(5 * time.Second)
				if err != nil {
					log.Println("unable to read message:", err.Error())
					_ = conn.Close()
					// 删除epoll事件
					if err := unix.EpollCtl(epollFd, syscall.EPOLL_CTL_DEL, int(fd), nil); err != nil {
						log.Println("unable to remove event")
					}
				}
				// 给客户端回消息
				if err := conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("receive:%s", string(message)))); err != nil {
					log.Println("unable to send message:", err.Error())
				}
			}
		}
	}()
	// 开启一个协程监听epoll事件
	go func() {
		for {
			// 声明50个events，表明每次最多获取50个事件
			events := make([]unix.EpollEvent, 50)
			// 每100ms执行一次
			//n, err := unix.EpollWait(epollFd, events, 100)
			n, err := unix.EpollWait(epollFd, events, -1)
			if err != nil {
				log.Println("vepoll wait:", err.Error())
				continue
			}

			// 取出来的是就绪的websocket连接的fd
			for i := 0; i < n; i++ {
				if events[i].Fd == 0 {
					continue
				}
				fmt.Println("notify fd:", events[i].Fd)
				ch <- events[i].Fd // 通过channel传递到另一个goroutine处理
			}
		}

	}()
	// 绑定http服务
	http.ListenAndServe(":8081", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 升级为websocket连接
		conn, err := NewWebsocketConnection(w, r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		// 获取文件标识符
		fd := GetSocketFD(conn.UnderlyingConn())
		// 注册事件
		if err := unix.EpollCtl(epollFd,
			unix.EPOLL_CTL_ADD,
			int(fd),
			&unix.EpollEvent{Events: unix.EPOLLIN | unix.EPOLLET, Fd: fd}); err != nil {
			log.Println("unable to add event:%v", err.Error())
			return
		}
		// 保存到map里
		mu.Lock()
		connections[fd] = conn
		mu.Unlock()
	}))
}

var u = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }} // use default options
// WebsocketConn return web socket connection
func NewWebsocketConnection(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	return u.Upgrade(w, r, nil)

}

// GetSocketFD get socket connection fd
func GetSocketFD(conn net.Conn) int32 {
	tcpConn := reflect.Indirect(reflect.ValueOf(conn)).FieldByName("conn")
	fdVal := tcpConn.FieldByName("fd")
	pfdVal := reflect.Indirect(fdVal).FieldByName("pfd")
	return int32(pfdVal.FieldByName("Sysfd").Int())
}
