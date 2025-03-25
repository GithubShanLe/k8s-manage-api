package terminal

import (
	"bufio" // 添加 bufio 包的导入
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/gorilla/websocket"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// CommandRequest 定义命令请求结构体
type CommandRequest struct {
	Command string `json:"command"`
}

// CommandResponse 定义命令响应结构体
type CommandResponse struct {
	Output string `json:"output"`
	Dir    string `json:"dir"` // 新增字段，用于返回当前目录
	Error  string `json:"error"`
}

// 定义 WebSocket 升级器
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ExecuteCommandHandler 处理执行命令的 WebSocket 请求和普通 HTTP 请求
func ExecuteCommandHandler(w http.ResponseWriter, r *http.Request) {
	var currentDir string // 用于存储当前工作目录
	if r.Header.Get("Upgrade") == "websocket" {
		// 处理 WebSocket 请求
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("WebSocket 升级失败:", err)
			return
		}
		defer conn.Close()

		for {
			// 读取 WebSocket 消息
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					log.Printf("WebSocket 连接异常关闭: %v", err)
				} else {
					log.Println("读取消息失败:", err)
				}
				break
			}
			log.Printf("Received WebSocket message: type=%d, message=%s", messageType, string(message)) // Add debugging statement

			var req CommandRequest
			err = json.Unmarshal(message, &req)
			if err != nil {
				resp := CommandResponse{
					Error: "请求体解析失败",
				}
				respJSON, _ := json.Marshal(resp)
				conn.WriteMessage(websocket.TextMessage, respJSON)
				continue
			}

			// 解析命令和参数
			parts := strings.Fields(req.Command)
			if len(parts) == 0 {
				resp := CommandResponse{
					Error: "无效的命令",
				}
				respJSON, _ := json.Marshal(resp)
				conn.WriteMessage(websocket.TextMessage, respJSON)
				continue
			}
			cmdName := parts[0]
			cmdArgs := parts[1:]

			if cmdName == "cd" {
				if len(cmdArgs) == 0 {
					resp := CommandResponse{
						Error: "cd 命令缺少目标目录",
					}
					respJSON, _ := json.Marshal(resp)
					conn.WriteMessage(websocket.TextMessage, respJSON)
					continue
				}
				targetDir := cmdArgs[0]
				err := os.Chdir(targetDir)
				if err != nil {
					resp := CommandResponse{
						Error: fmt.Sprintf("无法切换到目录 %s: %v", targetDir, err),
					}
					respJSON, _ := json.Marshal(resp)
					conn.WriteMessage(websocket.TextMessage, respJSON)
					continue
				}
				currentDir = targetDir
				resp := CommandResponse{
					Output: fmt.Sprintf("已切换到目录 %s", targetDir),
					Dir:    currentDir, // 更新当前目录
				}
				respJSON, _ := json.Marshal(resp)
				conn.WriteMessage(websocket.TextMessage, respJSON)
				continue
			}

			// 执行命令
			cmd := exec.Command(cmdName, cmdArgs...)
			if currentDir != "" {
				cmd.Dir = currentDir
			}

			// 获取命令的标准输出和标准错误输出管道
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				log.Println("获取标准输出管道失败:", err)
				break
			}
			stderr, err := cmd.StderrPipe()
			if err != nil {
				log.Println("获取标准错误输出管道失败:", err)
				break
			}

			// 启动命令
			if err := cmd.Start(); err != nil {
				resp := CommandResponse{
					Error: fmt.Sprintf("启动命令失败: %v", err),
					Dir:   currentDir,
				}
				respJSON, _ := json.Marshal(resp)
				conn.WriteMessage(websocket.TextMessage, respJSON)
				continue
			}

			// 实时读取标准输出和标准错误输出，处理编码
			go func() {
				reader := bufio.NewReader(stdout)
				for {
					line, err := reader.ReadString('\n')
					if line != "" {
						// 进行编码转换
						line, err = convertToUTF8(line)
						if err != nil {
							log.Println("编码转换失败:", err)
						}
						resp := CommandResponse{
							Output: line,
							Dir:    currentDir,
						}
						respJSON, _ := json.Marshal(resp)
						conn.WriteMessage(websocket.TextMessage, respJSON)
					}
					if err != nil {
						if err != io.EOF {
							log.Println("读取标准输出失败:", err)
						}
						break
					}
				}
			}()

			go func() {
				buf := make([]byte, 1024)
				for {
					n, err := stderr.Read(buf)
					if n > 0 {
						// 进行编码转换
						output, err := convertToUTF8(string(buf[:n]))
						if err != nil {
							log.Println("编码转换失败:", err)
						}
						resp := CommandResponse{
							Output: output,
							Dir:    currentDir,
							Error:  output,
						}
						respJSON, _ := json.Marshal(resp)
						conn.WriteMessage(websocket.TextMessage, respJSON)
					}
					if err != nil {
						if err != io.EOF {
							log.Println("读取标准错误输出失败:", err)
						}
						break
					}
				}
			}()

			// 等待命令执行完成
			if err := cmd.Wait(); err != nil {
				resp := CommandResponse{
					Error: fmt.Sprintf("命令执行失败: %v", err),
					Dir:   currentDir,
				}
				respJSON, _ := json.Marshal(resp)
				conn.WriteMessage(websocket.TextMessage, respJSON)
			}

			if currentDir == "" {
				currentDir, _ = os.Getwd()
			}
		}
	} else {
		// 处理普通 HTTP 请求
		if r.Method != http.MethodPost {
			http.Error(w, "只支持 POST 请求", http.StatusMethodNotAllowed)
			return
		}

		var req CommandRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			http.Error(w, "请求体解析失败", http.StatusBadRequest)
			return
		}

		// 解析命令和参数
		parts := strings.Fields(req.Command)
		if len(parts) == 0 {
			http.Error(w, "无效的命令", http.StatusBadRequest)
			return
		}
		cmdName := parts[0]
		cmdArgs := parts[1:]

		if cmdName == "cd" {
			if len(cmdArgs) == 0 {
				http.Error(w, "cd 命令缺少目标目录", http.StatusBadRequest)
				return
			}
			targetDir := cmdArgs[0]
			err := os.Chdir(targetDir)
			if err != nil {
				http.Error(w, fmt.Sprintf("无法切换到目录 %s: %v", targetDir, err), http.StatusBadRequest)
				return
			}
			currentDir = targetDir
			resp := CommandResponse{
				Output: fmt.Sprintf("已切换到目录 %s", targetDir),
				Dir:    currentDir, // 更新当前目录
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// 执行命令
		cmd := exec.Command(cmdName, cmdArgs...)
		if currentDir != "" {
			cmd.Dir = currentDir
		}
		output, err := cmd.CombinedOutput()

		if currentDir == "" {
			currentDir, _ = os.Getwd()
		}

		resp := CommandResponse{
			Output: string(output),
			Dir:    currentDir, // 返回当前目录
		}
		if err != nil {
			resp.Error = err.Error()
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// convertToUTF8 将字符串从 GBK 转换为 UTF-8
func convertToUTF8(s string) (string, error) {
	reader := transform.NewReader(strings.NewReader(s), simplifiedchinese.GBK.NewDecoder())
	b, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
