package terminal

import (
	"fmt"
	"io"
	"k8s-manage-api/k8s"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
)

type PodLogSession struct {
	ws     *websocket.Conn
	closed bool
	mutex  sync.Mutex
}

func (l *PodLogSession) Close() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if !l.closed {
		l.closed = true
		l.ws.Close()
	}
}

func PodLogs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// 获取查询参数
	params := r.URL.Query()
	namespace := params.Get("namespace")
	podName := params.Get("podName")
	containerName := params.Get("containerName")
	previous   := params.Get("previous")
	tailLines, _ := strconv.ParseInt(params.Get("tailLines"), 10, 64)
	if tailLines == 0 {
		tailLines = 1000
	}
	p := false
	if previous != "" {
		p = true
	}
	// 参数校验
	if namespace == "" || podName == "" {
		conn.WriteJSON(map[string]string{"error": "missing required parameters"})
		return
	}
	fmt.Println("namespace: ", namespace, "podName: ", podName, "containerName: ", containerName, "previous: ", previous, "tailLines: ", tailLines)

	session := &PodLogSession{ws: conn}
	defer session.Close()

	clientset := k8s.GetClient()
	
	// 配置日志选项
	podLogOpts := &corev1.PodLogOptions{
		Container:    containerName,
		Follow:       true,
		Timestamps:   false,
		TailLines:    &tailLines,
		Previous:     p,
	}

	// 创建日志流
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, podLogOpts)
	stream, err := req.Stream(r.Context())
	if err != nil {
		session.sendError(fmt.Sprintf("failed to get log stream: %v", err))
		return
	}
	defer stream.Close()

	// 实时传输日志
	go session.streamLogs(stream)

	// 保持连接
	session.keepAlive()
}

func (l *PodLogSession) streamLogs(stream io.ReadCloser) {
	buf := make([]byte, 4096)
	for {
		n, err := stream.Read(buf)
		if err != nil {
			if err != io.EOF {
				l.sendError(fmt.Sprintf("log read error: %v", err))
			}
			return
		}

		l.mutex.Lock()
		if l.closed {
			l.mutex.Unlock()
			return
		}

		if err := l.ws.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
			l.mutex.Unlock()
			return
		}
		l.mutex.Unlock()
	}
}

func (l *PodLogSession) keepAlive() {
	for {
		if _, _, err := l.ws.NextReader(); err != nil {
			break
		}
	}
}

func (l *PodLogSession) sendError(msg string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if !l.closed {
		l.ws.WriteJSON(map[string]string{"error": msg})
	}
}