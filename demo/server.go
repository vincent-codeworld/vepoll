package main

import (
	"encoding/json"
	"epoll/vepoll"
	"fmt"
	"github.com/gorilla/websocket"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"time"
)

var webSocketMap sync.Map

//var Upgrader = websocket.HertzUpgrader{
//	ReadBufferSize:  4 * 1024, // 读缓冲区大小
//	WriteBufferSize: 4 * 1024, // 写缓冲区大小
//	CheckOrigin: func(ctx *app.RequestContext) bool {
//		return true
//	},
//	// 以下选项对 Netpoll 模式特别重要
//	EnableCompression: false, // 通常禁用压缩以提高性能
//}

var Upgrader = websocket.Upgrader{
	CheckOrigin:       func(r *http.Request) bool { return true },
	ReadBufferSize:    4 * 1024, // 读缓冲区大小
	WriteBufferSize:   4 * 1024, // 写缓冲区大小
	EnableCompression: false,    // 通常禁用压缩以提高性能
}

type Message struct {
	MsgId       string `json:"msg_id"`
	FrmUserId   string `json:"frm_user_id"`
	ToUserId    string `json:"to_user_id"`
	MessageType int    `json:"message_type"`
	Content     string `json:"content"`
	CreatedAt   uint64 `json:"created_at"`
}

func (message *Message) ConverMessage(msg []byte) {
	//todo 使用其他序列化框架提升性能
	json.Unmarshal(msg, message)
}

var vpoll *vepoll.VePoll

func main() {
	go func() {
		startWebSocketServer()
	}()
	var err error
	vpoll, err = vepoll.NewVePoll(vepoll.WebSocketMode)
	if err != nil {
		panic(err)
	}

	vpoll.SetHandler(func(message []byte) error {
		msg := new(Message)
		msg.ConverMessage(message)
		fdV, ok := webSocketMap.Load(msg.ToUserId)
		if !ok {
			slog.Warn(fmt.Sprintf("ws,user id: %s, can not found", msg.ToUserId))
			return nil
		}
		fd := fdV.(int32)
		ws := vpoll.GetWebSocket(fd)
		if ws == nil {
			slog.Warn(fmt.Sprintf("ws,user id: %s, can not found", msg.ToUserId))
			return nil
		}
		_ = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
		w, _ := ws.NextWriter(websocket.TextMessage)
		_, _ = w.Write([]byte(msg.Content))
		if err := w.Close(); err != nil {
			slog.Error("NextWriter Close err: %+v", err)
			return err
		}
		return nil
	})
	vpoll.Run()
}

var imHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	userId := query.Get("user_id")
	// 升级为websocket连接
	slog.Info(fmt.Sprintf("use id, ws connected: %s", userId))
	conn, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	fd, _ := vpoll.AddOuterWebSocket(conn)
	webSocketMap.Store(userId, fd)
})

//func startWebSocketServer() {
//	h := server.Default(
//		server.WithHostPorts("0.0.0.0:8081"),
//	)
//	h.GET("/im", func(c context.Context, ctx *app.RequestContext) {
//		userId := strings.TrimSpace(ctx.Query("user_id"))
//		if userId == "" {
//			ctx.SetStatusCode(http.StatusBadRequest)
//			return
//		}
//		ServeIm(ctx, vpoll)
//	})
//	h.Spin()
//}

func startWebSocketServer() {
	http.HandleFunc("/im", imHandler)
	// 绑定http服务
	http.ListenAndServe(":8081", nil)
}

//func startWebSocketServerTest() {
//	h := server.Default(
//		server.WithHostPorts("0.0.0.0:8081"),
//	)
//	h.GET("/im", func(c context.Context, ctx *app.RequestContext) {
//		userId := strings.TrimSpace(ctx.Query("user_id"))
//		slog.Info("userid", userId)
//	})
//	h.Spin()
//}
