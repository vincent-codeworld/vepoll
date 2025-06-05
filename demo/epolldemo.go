package main

//
//
//import (
//	"fmt"
//	"github.com/gorilla/websocket"
//	"golang.org/x/sys/unix"
//	"log"
//	"log/slog"
//	"net"
//	"net/http"
//	_ "net/http/pprof"
//	"reflect"
//	"sync"
//	"syscall"
//)
//
//func main() {
//	// 创建epoll
//	epollFd, err := unix.EpollCreate(1)
//	if err != nil {
//		log.Fatalf("unable to create vepoll:%v\n", err)
//	}
//
//	connections := make(map[int32]*websocket.Conn, 1000000)
//	var mu sync.Mutex
//	ch := make(chan int32, 50)
//
//	// 开启协程，处理接受到的信息
//	go func() {
//		for {
//			select {
//			case fd := <-ch:
//				//slog.Info(fmt.Sprintf("get message from fd: %d", fd))
//				fmt.Printf("get message from fd: %d \n", fd)
//				conn := connections[fd]
//				if conn == nil {
//					slog.Warn(fmt.Sprintf("could not get connection, fd: %d", fd))
//					continue
//				}
//				// 接收消息
//				_, message, err := conn.ReadMessage()
//				fmt.Printf("message: %s \n", message)
//				if err != nil {
//					log.Println("unable to read message:", err.Error())
//					_ = conn.Close()
//					// 删除epoll事件
//					if err := unix.EpollCtl(epollFd, syscall.EPOLL_CTL_DEL, int(fd), nil); err != nil {
//						log.Println("unable to remove event")
//					}
//				}
//				// 给客户端回消息
//				if err := conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("receive:%s", string(message)))); err != nil {
//					log.Println("unable to send message:", err.Error())
//				}
//			}
//		}
//	}()
//	// 开启一个协程监听epoll事件
//	go func() {
//		for {
//			// 声明50个events，表明每次最多获取50个事件
//			events := make([]unix.EpollEvent, 10)
//			// 每100ms执行一次
//			//n, err := unix.EpollWait(epollFd, events, 100)
//			n, err := unix.EpollWait(epollFd, events, -1)
//			if err != nil {
//				//log.Println("vepoll wait:", err.Error())
//				slog.Error("vepoll wait:", err.Error())
//				continue
//			}
//			//slog.Info(fmt.Sprintf("events: %+v,n: %d", events, n))
//			fmt.Printf(fmt.Sprintf("events: %+v,n: %d \n", events, n))
//			// 取出来的是就绪的websocket连接的fd
//			for i := 0; i < n; i++ {
//				if events[i].Fd == 0 {
//					//slog.Info("event: %+v, fd is 0", events[i])
//					fmt.Printf("event: %+v, fd is 0 \n", events[i])
//					continue
//				}
//				//slog.Info("pass event: %+v", events[i])
//				fmt.Printf("pass event: %+v \n", events[i])
//				ch <- events[i].Fd // 通过channel传递到另一个goroutine处理
//			}
//		}
//
//	}()
//
//	imHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		// 升级为websocket连接
//		conn, err := NewWebsocketConnection(w, r)
//		if err != nil {
//			http.NotFound(w, r)
//			return
//		}
//		// 获取文件标识符
//		fd := GetSocketFD(conn.UnderlyingConn())
//		slog.Info(fmt.Sprintf("get new connection,fd: %d", fd))
//		// 注册事件
//		if err := unix.EpollCtl(epollFd,
//			unix.EPOLL_CTL_ADD,
//			int(fd),
//			&unix.EpollEvent{Events: unix.EPOLLIN | unix.EPOLLET, Fd: fd}); err != nil {
//			slog.Error(fmt.Sprintf("unable to add event:%v", err.Error()))
//			return
//		}
//		// 保存到map里
//		mu.Lock()
//		connections[fd] = conn
//		mu.Unlock()
//	})
//	http.HandleFunc("/im", imHandler)
//	// 绑定http服务
//	http.ListenAndServe(":8081", nil)
//}
//
//var u = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }} // use default options
//// WebsocketConn return web socket connection
//func NewWebsocketConnection(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
//	return u.Upgrade(w, r, nil)
//
//}
//
//// GetSocketFD get socket connection fd
//func GetSocketFD(conn net.Conn) int32 {
//	tcpConn := reflect.Indirect(reflect.ValueOf(conn)).FieldByName("conn")
//	fdVal := tcpConn.FieldByName("fd")
//	pfdVal := reflect.Indirect(fdVal).FieldByName("pfd")
//	return int32(pfdVal.FieldByName("Sysfd").Int())
//}
