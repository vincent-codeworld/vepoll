package manager

import (
	"fmt"
)

//func NewPoll(listenFD int) (*epoll, error) {
//	// epoll fd close when executing a commnd function 'exec'
//	fd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
//	if err != nil {
//		return nil, err
//	}
//
//}

func Printtt(s string) {
	fmt.Println(s)
}
