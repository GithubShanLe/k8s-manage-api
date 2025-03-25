package terminal

import (
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type TerminalSession struct {
	ws     *websocket.Conn
	pty    *os.File
	cmd    *exec.Cmd
	closed bool
	mutex  sync.Mutex
}

// 处理终端 WebSocket 连接
func HandleTerminal(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("升级 WebSocket 连接失败: %v\n", err)
		return
	}
	defer conn.Close()

	// 创建终端会话
	session, err := createTerminalSession()
	if err != nil {
		log.Printf("创建终端会话失败: %v\n", err)
		return
	}
	defer session.Close()

	session.ws = conn

	// 启动数据传输
	go session.readFromWebSocket()
	session.writeToWebSocket()
}

func createTerminalSession() (*TerminalSession, error) {
	// 设置命令和相关属性
	cmd := exec.Command("/bin/sh")
	cmd.Env = append(os.Environ(), "TERM=xterm")

	// 完全移除 SysProcAttr 的设置
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	// 设置更大的终端初始大小
	_ = pty.Setsize(ptmx, &pty.Winsize{
		Rows: 40,
		Cols: 120,
	})

	return &TerminalSession{
		pty: ptmx,
		cmd: cmd,
	}, nil
}

func (t *TerminalSession) readFromWebSocket() {
	for {
		_, data, err := t.ws.ReadMessage()
		if err != nil {
			break
		}

		t.mutex.Lock()
		if !t.closed {
			_, err = t.pty.Write(data)
			if err != nil {
				log.Printf("写入终端失败: %v\n", err)
			}
		}
		t.mutex.Unlock()
	}
}

func (t *TerminalSession) writeToWebSocket() {
	buf := make([]byte, 1024)
	for {
		n, err := t.pty.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("读取终端输出失败: %v\n", err)
			}
			break
		}

		t.mutex.Lock()
		if !t.closed {
			err = t.ws.WriteMessage(websocket.BinaryMessage, buf[:n])
			if err != nil {
				log.Printf("发送消息到 WebSocket 失败: %v\n", err)
				break
			}
		}
		t.mutex.Unlock()
	}
}

func (t *TerminalSession) Close() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.closed {
		t.closed = true
		t.pty.Close()
		t.cmd.Process.Kill()
		t.ws.Close()
	}
}
