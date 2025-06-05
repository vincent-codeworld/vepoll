package main

import (
	"github.com/gorilla/websocket"
	"log"
	"net/url"
	"time"
)

func main() {
	// 捕获 OS 中断信号，用于优雅退出
	//interrupt := make(chan os.Signal, 1)
	//signal.Notify(interrupt, os.Interrupt)

	// 构造 WebSocket 的 URL，可根据需要加入查询参数
	u := url.URL{
		Scheme:   "ws",
		Host:     "localhost:8081",
		Path:     "/im",
		RawQuery: "user_id=123", // 根据实际情况填写 user_id
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				_ = conn.Close()
				log.Fatalf("dial error: %v", err)
			}
			// 可打印响应状态和头信息以作调试
			log.Println("connected, response status:", resp.Status)
		}
	}
}
