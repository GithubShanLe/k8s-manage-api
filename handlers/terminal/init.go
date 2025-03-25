package terminal

import (
	"fmt"
	"log"
	"net/http"
)

func init() {
	go StartServer(9000)
}

// StartServer 启动 WebSocket 和 HTTP 服务器
func StartServer(port int) {
	http.HandleFunc("/execute/shell", ExecuteCommandHandler)
	http.HandleFunc("/execute/podshell", PodExec)
	log.Printf("WebSocket 和 HTTP API 服务器启动，监听端口: %d\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
