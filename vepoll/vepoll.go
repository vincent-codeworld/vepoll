//go:build linux

package vepoll

import (
	"context"
	"epoll/consts"
	"fmt"
	"github.com/hertz-contrib/websocket"
	"golang.org/x/sys/unix"
	"log/slog"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
)

const (
	TcpMode = iota + 1
	WebSocketMode
)

type VePoll struct {
	mode        int
	mu          sync.RWMutex
	fd          int
	connections map[int32]net.Conn
	webSockets  map[int32]*websocket.Conn
	events      []unix.EpollEvent
	cancelCtx   context.Context
	cancelFun   context.CancelFunc
	handler     func(message []byte) error
	status      int
}

func NewVePoll() (*VePoll, error) {
	fd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	vpl := &VePoll{
		fd:          fd,
		connections: make(map[int32]net.Conn, 1024),
		status:      consts.VePollStatusUnknown,
		events:      make([]unix.EpollEvent, 100),
	}
	vpl.cancelCtx, vpl.cancelFun = context.WithCancel(context.Background())
	return vpl, nil
}

func (vpl *VePoll) addConnToEpoll(file *os.File) error {
	err := unix.EpollCtl(vpl.fd, unix.EPOLL_CTL_ADD, int(file.Fd()), &unix.EpollEvent{
		Events: unix.EPOLLIN | unix.EPOLLET, // 边缘触发 + 可读事件
		Fd:     int32(file.Fd()),
	})
	if err != nil {
		err = fmt.Errorf("calling EpollCtl err: %+v", err)
		_ = file.Close()
		slog.Error(err.Error())
		return err
	}
	return nil
}

func (vpl *VePoll) AddTcpConn(conn net.Conn) error {
	vpl.mu.Lock()
	defer vpl.mu.Unlock()
	file, err := socketFD(conn)
	if err != nil {
		slog.Error("err", err)
		return err
	}
	fd := int32(file.Fd())
	// 设置非阻塞模式
	if err := syscall.SetNonblock(int(fd), true); err != nil {
		_ = file.Close()
		return fmt.Errorf("set connection non block, err: %v", err)
	}

	if err = vpl.addConnToEpoll(file); err != nil {
		return err
	}
	vpl.connections[fd] = conn
	return nil
}

func (vpl *VePoll) AddOuterWebSocket(conn *websocket.Conn) error {
	vpl.mu.Lock()
	defer vpl.mu.Unlock()
	file, err := socketFD(conn.NetConn())
	if err != nil {
		slog.Error("err", err)
		return err
	}
	if err = vpl.addConnToEpoll(file); err != nil {
		return err
	}
	vpl.webSockets[int32(file.Fd())] = conn
	return nil
}

// 从 vepoll 移除 socket
func (vpl *VePoll) Remove(fd int32) error {
	vpl.mu.Lock()
	defer vpl.mu.Unlock()
	f := func(fd int32) error {
		err := unix.EpollCtl(vpl.fd, unix.EPOLL_CTL_DEL, int(fd), nil)
		if err != nil {
			err = fmt.Errorf("calling EpollCtl err: %+v", err)
			slog.Error(err.Error())
			return err
		}
		return nil
	}
	switch vpl.mode {
	case TcpMode:
		{
			conn, ok := vpl.connections[fd]
			if !ok {
				err := fmt.Errorf("fd does not exist, fd: %d", fd)
				return err
			}
			if err := f(fd); err != nil {
				return err
			}
			_ = conn.Close()
			delete(vpl.connections, fd)
		}
	case WebSocketMode:
		{
			conn, ok := vpl.webSockets[fd]
			if !ok {
				err := fmt.Errorf("fd does not exist, fd: %d", fd)
				return err
			}
			if err := f(fd); err != nil {
				return err
			}
			_ = conn.Close()
			delete(vpl.webSockets, fd)
		}
	}

	slog.Info("VePoll Remove fd", fd)
	return nil
}

func (vpl *VePoll) Run() error {
	switch vpl.mode {
	case TcpMode:
		return vpl.runTcp()
	case WebSocketMode:
		return vpl.runWebsocket()
	}
	return fmt.Errorf("can not support other mode: %d", vpl.mode)
}

func (vpl *VePoll) Stop() {
	vpl.cancelFun()
}

func (vpl *VePoll) runTcp() error {
	panic(fmt.Errorf("can not support tcp mode"))
}

func (vpl *VePoll) runWebsocket() error {
	cancelSign := vpl.cancelCtx.Done()
	sign := make(chan struct{}, 1)

	for {
		select {
		case <-cancelSign:
			slog.Warn("closing vepoll")
			return nil
		case <-vpl.waitWebsocket(sign):
		}
	}
}

func (vpl *VePoll) waitWebsocket(sign chan struct{}) <-chan struct{} {
	n, err := unix.EpollWait(vpl.fd, vpl.events, -1)
	if err != nil {
		slog.Error("epoll waitWebsocket", err.Error())
		time.Sleep(3 * time.Second)
		sign <- struct{}{}
		return sign
	}
	// 取出来的是就绪的websocket连接的fd
	for i := 0; i < n; i++ {
		fd := vpl.events[i].Fd
		if fd == 0 {
			continue
		}
		vpl.mu.RLock()
		conn := vpl.webSockets[fd]
		vpl.mu.RUnlock()
		if conn == nil {
			continue
		}
		_, message, err := conn.ReadMessage()
		if err != nil {
			slog.Error("unable to read message,err", err.Error())
			_ = conn.Close()
			// 删除epoll事件
			if err := unix.EpollCtl(vpl.fd, syscall.EPOLL_CTL_DEL, int(fd), nil); err != nil {
				slog.Error("unable to remove event,err", err)
			}
			continue
		}
		_ = vpl.handler(message)
	}
	sign <- struct{}{}
	return sign
}

// 获取 socket 的文件描述符
func socketFD(conn net.Conn) (*os.File, error) {
	tcpConn := conn.(*net.TCPConn)
	file, err := tcpConn.File()
	if err != nil {
		slog.Error("err", err)
		return nil, err
	}
	return file, nil
}
