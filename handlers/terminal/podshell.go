package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s-manage-api/k8s"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// WSClient 实现了 remotecommand 所需的接口
type WSClient struct {
	ws     *websocket.Conn
	resize chan remotecommand.TerminalSize
	done   chan struct{} // 用于通知关闭
}

// MSG WebSocket 消息结构
type MSG struct {
	MsgType string `json:"msg_type"`
	Rows    uint16 `json:"rows"`
	Cols    uint16 `json:"cols"`
	Data    string `json:"data"`
}

// PodExecRequest Pod 执行命令请求结构
type PodExecRequest struct {
	Namespace     string `json:"namespace"`
	PodName       string `json:"podName"`
	ContainerName string `json:"containerName"`
	Command       string `json:"command"`
}

// Read 实现 io.Reader 接口
func (c *WSClient) Read(p []byte) (n int, err error) {
	_, message, err := c.ws.ReadMessage()
	if err != nil {
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
			log.Printf("WebSocket 连接异常关闭: %v", err)
		}
		close(c.done) // 通知其他协程连接已关闭
		return 0, err
	}

	var msg MSG
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Printf("解析消息失败: %v", err)
		return 0, err
	}

	switch msg.MsgType {
	case "resize":
		winSize := remotecommand.TerminalSize{
			Width:  msg.Cols,
			Height: msg.Rows,
		}
		// 非阻塞发送，避免死锁
		select {
		case c.resize <- winSize:
		default:
			log.Println("调整大小通道已满，跳过此次调整")
		}
		return 0, nil
	case "input":
		copy(p, msg.Data)
		return len(msg.Data), nil
	default:
		log.Printf("未知消息类型: %s", msg.MsgType)
		return 0, nil
	}
}

// Write 实现 io.Writer 接口
func (c *WSClient) Write(p []byte) (n int, err error) {
	select {
	case <-c.done:
		return 0, fmt.Errorf("连接已关闭")
	default:
		err = c.ws.WriteMessage(websocket.TextMessage, p)
		if err != nil {
			log.Printf("写入消息失败: %v", err)
			return 0, err
		}
		return len(p), nil
	}
}

// Next 实现 TerminalSizeQueue 接口
func (c *WSClient) Next() *remotecommand.TerminalSize {
	select {
	case size := <-c.resize:
		return &size
	case <-c.done:
		return nil
	}
}

// PodExec 处理 Pod 执行命令的 WebSocket 请求
func PodExec(w http.ResponseWriter, r *http.Request) {
	log.Println("开始处理 Pod Shell 请求")

	// 升级 HTTP 连接为 WebSocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket 升级失败: %v", err)
		http.Error(w, fmt.Sprintf("WebSocket升级失败: %v", err), http.StatusInternalServerError)
		return
	}
	defer ws.Close()

	// 设置 WebSocket 连接参数
	ws.SetReadDeadline(time.Now().Add(time.Hour * 24)) // 设置较长的超时时间
	ws.SetWriteDeadline(time.Now().Add(time.Hour * 24))

	// 初始化 WSClient
	wsClient := &WSClient{
		ws:     ws,
		resize: make(chan remotecommand.TerminalSize, 10), // 缓冲区避免阻塞
		done:   make(chan struct{}),
	}
	defer close(wsClient.done)

	// 从 WebSocket 消息中读取请求，而不是从 HTTP 请求体中读取
	_, message, err := ws.ReadMessage()
	if err != nil {
		log.Printf("读取 WebSocket 消息失败: %v", err)
		sendErrorMessage(ws, fmt.Sprintf("读取请求失败: %v", err))
		return
	}

	// 解析请求体
	var req PodExecRequest
	if err := json.Unmarshal(message, &req); err != nil {
		log.Printf("解析请求体失败: %v", err)
		sendErrorMessage(ws, fmt.Sprintf("解析请求失败: %v", err))
		return
	}

	log.Printf("执行 Pod 命令: namespace=%s, pod=%s, container=%s, command=%s",
		req.Namespace, req.PodName, req.ContainerName, req.Command)

	// 获取 Kubernetes 客户端
	clientset := k8s.GetClient()

	// 构建请求
	request := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(req.Namespace).
		Name(req.PodName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: req.ContainerName,
			Command:   []string{"/bin/sh", "-c", req.Command},
			Stdout:    true,
			Stdin:     true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	// 创建 SPDY 执行器
	exec, err := remotecommand.NewSPDYExecutor(k8s.GetRestConfig(), "POST", request.URL())
	if err != nil {
		log.Printf("创建 SPDY 执行器失败: %v", err)
		sendErrorMessage(ws, fmt.Sprintf("连接到 Pod 失败: %v", err))
		return
	}

	// 使用上下文控制执行流程
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动一个协程监听 WebSocket 关闭
	go func() {
		select {
		case <-wsClient.done:
			log.Println("WebSocket 连接已关闭，取消命令执行")
			cancel()
		case <-ctx.Done():
			return
		}
	}()

	// 执行命令
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stderr:            wsClient,
		Stdout:            wsClient,
		Stdin:             wsClient,
		Tty:               true,
		TerminalSizeQueue: wsClient,
	})

	if err != nil {
		log.Printf("命令执行失败: %v", err)
		sendErrorMessage(ws, fmt.Sprintf("命令执行失败: %v", err))
		return
	}

	log.Println("Pod Shell 会话正常结束")
}

// sendErrorMessage 发送错误消息到 WebSocket
func sendErrorMessage(ws *websocket.Conn, errMsg string) {
	resp := struct {
		Error string `json:"error"`
	}{
		Error: errMsg,
	}

	jsonResp, _ := json.Marshal(resp)
	ws.WriteMessage(websocket.TextMessage, jsonResp)
}
