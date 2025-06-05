//go:build linux

package vepoll

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/vincent-codeworld/vepoll/consts"
	"golang.org/x/sys/unix"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
)

const (
	TcpMode = iota + 1
	WebSocketMode
)

type VePoll struct {
	mode         int
	mu           sync.RWMutex
	fd           int
	connections  map[int32]net.Conn
	webSockets   map[int32]*websocket.Conn
	events       []unix.EpollEvent
	cancelCtx    context.Context
	cancelFun    context.CancelFunc
	writeHandler func(message any) error
	readHandler  func(message []byte) (any, bool, error)
	writeCh      chan any
	status       int
}

func NewVePoll(mode int) (*VePoll, error) {
	fd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	vpl := &VePoll{
		fd:          fd,
		mode:        mode,
		connections: make(map[int32]net.Conn, 1024),
		webSockets:  make(map[int32]*websocket.Conn, 1024),
		status:      consts.VePollStatusUnknown,
		events:      make([]unix.EpollEvent, 100),
		writeCh:     make(chan any, 1024),
	}
	vpl.cancelCtx, vpl.cancelFun = context.WithCancel(context.Background())
	return vpl, nil
}
func (vpl *VePoll) GetWebSocket(fd int32) *websocket.Conn {
	vpl.mu.RLock()
	defer vpl.mu.RUnlock()
	return vpl.webSockets[fd]
}
func (vpl *VePoll) SetWriteHandler(handler func(message any) error) {
	vpl.writeHandler = handler
}
func (vpl *VePoll) SetReadHandler(handler func(message []byte) (any, bool, error)) {
	vpl.readHandler = handler
}

func (vpl *VePoll) addConnToEpoll(fd int) error {
	err := unix.EpollCtl(vpl.fd, unix.EPOLL_CTL_ADD, fd, &unix.EpollEvent{
		Events: unix.EPOLLIN | unix.EPOLLET, // 边缘触发 + 可读事件
		Fd:     int32(fd),
	})
	if err != nil {
		err = fmt.Errorf("calling EpollCtl err: %+v", err)
		slog.Error(err.Error())
		return err
	}
	return nil
}

func (vpl *VePoll) AddTcpConn(conn net.Conn) error {
	vpl.mu.Lock()
	defer vpl.mu.Unlock()
	fd := getSocketFD(conn)
	slog.Info("add fd", fd)
	// 设置非阻塞模式
	if err := syscall.SetNonblock(int(fd), true); err != nil {
		//_ = file.Close()
		_ = conn.Close()
		return fmt.Errorf("set connection non block, err: %v", err)
	}

	if err := vpl.addConnToEpoll(int(fd)); err != nil {
		_ = conn.Close()
		return err
	}
	vpl.connections[fd] = conn
	return nil
}

func (vpl *VePoll) AddOuterWebSocket(conn *websocket.Conn) (int32, error) {
	vpl.mu.Lock()
	defer vpl.mu.Unlock()
	fd := getSocketFD(conn.NetConn())
	slog.Info("add fd", fd)
	if err := vpl.addConnToEpoll(int(fd)); err != nil {
		slog.Info("add fd err", err.Error())
		_ = conn.Close()
		return 0, err
	}
	vpl.webSockets[fd] = conn
	return fd, nil
}

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
				slog.Error("VePoll Remove, err", err)
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
				slog.Error("VePoll Remove, err", err)
			}
			_ = conn.Close()
			delete(vpl.webSockets, fd)
		}
	}

	slog.Info("VePoll Remove fd", fd)
	return nil
}

func (vpl *VePoll) Run() error {
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		<-sigChan
		signal.Stop(sigChan)
		vpl.Stop()
	}()
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
	ctx, _ := context.WithCancel(vpl.cancelCtx)
	cancelSign := vpl.cancelCtx.Done()
	//read goroutine
	go func() {
		for {
			select {
			case <-ctx.Done():
				break
			default:
				err := vpl.waitWebsocket()
				if err == syscall.EBADFD {
					break
				}
			}

		}
	}()
	// write goroutine
	go func() {
		for {
			select {
			case <-ctx.Done():
				break
			case msg := <-vpl.writeCh:
				err := vpl.writeHandler(msg)
				if err != nil {
					slog.Info("writeHandler message, err", err)
				}
			}
		}
	}()

	<-cancelSign
	slog.Info("closing vepoll...")
	_ = vpl.release()
	return nil
}

func (vpl *VePoll) release() error {
	err := unix.Close(vpl.fd)
	if err != nil {
		slog.Error("close vepoll fd, err", err)
	}
	vpl.mu.Lock()
	defer vpl.mu.Unlock()
	if vpl.mode == WebSocketMode {
		for _, conn := range vpl.webSockets {
			_ = conn.Close()
		}
	} else if vpl.mode == TcpMode {
		for _, conn := range vpl.connections {
			_ = conn.Close()
		}
	}
	return nil
}

func (vpl *VePoll) waitWebsocket() error {
	n, err := unix.EpollWait(vpl.fd, vpl.events, -1)
	if err != nil {
		slog.Error("vepoll waitWebsocket", err.Error())
		return err
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
			_ = vpl.Remove(fd)
			continue
		}
		var d any = message
		var isWriteAble bool
		if vpl.readHandler != nil {
			d, isWriteAble, err = vpl.readHandler(message)
			if err != nil {
				slog.Error("read handler message,err", err.Error())
				_ = vpl.Remove(fd)
				continue
			}
		}
		slog.Info("message", string(message))
		if vpl.writeHandler != nil && isWriteAble {
			vpl.writeCh <- d
		}
	}
	return nil
}

// 获取 socket 的文件描述符
func socketFD(conn net.Conn) (*os.File, error) {
	//tcpConn := conn.(*route.hijackConn)
	tcpConn := conn.(*net.TCPConn)
	file, err := tcpConn.File()
	if err != nil {
		slog.Error("err", err)
		return nil, err
	}
	return file, nil
}

func getSocketFD(conn net.Conn) int32 {
	tcpConn := reflect.Indirect(reflect.ValueOf(conn)).FieldByName("conn")
	fdVal := tcpConn.FieldByName("fd")
	pfdVal := reflect.Indirect(fdVal).FieldByName("pfd")
	return int32(pfdVal.FieldByName("Sysfd").Int())
}
