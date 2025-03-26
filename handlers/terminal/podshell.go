package terminal

import (
	"context"
	"fmt"
	"k8s-manage-api/k8s"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

type PodTerminalSession struct {
	ws       *websocket.Conn
	sizeChan chan remotecommand.TerminalSize
	doneChan chan struct{}
	closed   bool
	mutex    sync.Mutex
}

func (t *PodTerminalSession) Read(p []byte) (n int, err error) {
	_, data, err := t.ws.ReadMessage()
	if err != nil {
		t.Close()
		return 0, err
	}

	copy(p, data)
	return len(data), nil
}

func (t *PodTerminalSession) Write(p []byte) (n int, err error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.closed {
		return 0, fmt.Errorf("session closed")
	}

	err = t.ws.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (t *PodTerminalSession) Next() *remotecommand.TerminalSize {
	select {
	case size := <-t.sizeChan:
		return &size
	case <-t.doneChan:
		return nil
	}
}

func (t *PodTerminalSession) Close() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.closed {
		t.closed = true
		close(t.doneChan)
		t.ws.Close()
	}
}

func PodExec(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("升级 WebSocket 连接失败: %v\n", err)
		return
	}
	defer conn.Close()

	namespace := r.URL.Query().Get("namespace")
	podName := r.URL.Query().Get("podName")
	containerName := r.URL.Query().Get("containerName")

	if namespace == "" || podName == "" {
		log.Printf("参数错误: namespace=%s, podName=%s\n", namespace, podName)
		return
	}

	session := &PodTerminalSession{
		ws:       conn,
		sizeChan: make(chan remotecommand.TerminalSize, 1),
		doneChan: make(chan struct{}),
	}
	defer session.Close()

	clientset := k8s.GetClient()

	shell := getAvailableShell(clientset, namespace, podName, containerName)
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")

	execOptions := &corev1.PodExecOptions{
		Container: containerName,
		Command:   shell,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}

	req.VersionedParams(execOptions, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(k8s.GetRestConfig(), "POST", req.URL())
	if err != nil {
		log.Printf("创建执行器失败: %v\n", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-session.doneChan
		cancel()
	}()

	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             session,
		Stdout:            session,
		Stderr:            session,
		Tty:               true,
		TerminalSizeQueue: session,
	})

	if err != nil {
		log.Printf("执行命令失败: %v\n", err)
	}
}

func getAvailableShell(clientset *kubernetes.Clientset, namespace, podName, containerName string) []string {
	// 尝试检查常见的 shell 路径
	shells := []string{"/bin/bash", "/bin/sh"}
	for _, shell := range shells {
		// 使用 which 命令检查 shell 是否存在
		cmd := []string{"which", shell}
		if containerName != "" {
			cmd = append([]string{"container=" + containerName}, cmd...)
		}

		_, err := clientset.CoreV1().RESTClient().Post().
			Resource("pods").
			Name(podName).
			Namespace(namespace).
			SubResource("exec").
			VersionedParams(&corev1.PodExecOptions{
				Container: containerName,
				Command:   cmd,
				Stdout:    true,
				Stderr:    true,
			}, scheme.ParameterCodec).Do(context.Background()).Raw()

		if err == nil {
			return []string{shell}
		}
	}

	// 默认返回 /bin/sh
	return []string{"/bin/sh"}
}
