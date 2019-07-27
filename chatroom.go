package main

import (
	"net"
	"fmt"
	"sync"
	"io"
	"strings"
	"time"
)

//客户端结构体
type Client struct {
	addr string
	name string
	ch   chan string
}

//全局channel，负责广播数据的存储
var Message = make(chan string)
//全局map，存储在线的用户
var onLineMap = make(map[string]Client)

var rwMutex sync.RWMutex //同步map-- onLineMap

func main() {
	listener, err := net.Listen("tcp", "127.0.0.1:8880")
	if err != nil {
		fmt.Println("net.Listen err:", err)
		return
	}
	defer listener.Close()

	go manager()

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("net.Listen err:", err)
			continue
		}

		go handleConnect(conn)

	}
}

//监控全局管道Message中的数据，并遍历分发给每个客户端的channel
func manager() {
	for data := range Message {
		rwMutex.RLock()
		for _, clnt := range onLineMap {
			clnt.ch <- data
		}
		rwMutex.RUnlock()
	}
}

//处理每个连接的client
func handleConnect(conn net.Conn) {
	defer conn.Close()

	//保存成员到onlineMap
	clnt := Client{conn.RemoteAddr().String(), conn.RemoteAddr().String(), make(chan string)}
	rwMutex.Lock()
	onLineMap[clnt.addr] = clnt
	rwMutex.Unlock()

	//监控本客户端的channel并发送嗯给client
	go writeMsgToClient(clnt, conn)

	//广播上线信息
	Message<- makeMsg(clnt, "login")

	isQuit := make(chan bool)
	isLive := make(chan bool)

	//读client数据
	go func() {
		buff := make([]byte, 4096)
		for {
			n, err := conn.Read(buff)
			if n == 0 {
				fmt.Println("clnt quit")
				isQuit<- true
				return
			}
			if err != nil && err != io.EOF {
				fmt.Println("conn.Read err:",err)
				isQuit<- true
				return
			}

			msg := buff[:n-1]
			if string(msg) == "who" {
				rwMutex.RLock()
				for _, clnt := range onLineMap {
					OnLineMsg := makeMsg(clnt, "[online]\n")
					conn.Write([]byte(OnLineMsg))
				}
				rwMutex.RUnlock()

			}else if len(msg) > 7 && string(msg[:7]) == "rename|" {
				//“短路运算”，不能将string(msg[:7]) == "rename|"放在&&前面，防止内存越界运行出错！
				newName := strings.Split(string(msg), "|")[1]
				clnt.name = newName

				//更新onlineMap
				rwMutex.Lock()
				onLineMap[clnt.addr] = clnt
				rwMutex.Unlock()

				//反馈给当前用户
				conn.Write([]byte("rename successful"))

			}else {
				Message <- makeMsg(clnt, string(buff[:n]))
			}

			isLive <- true

		}
	}()

	for {
		//runtime.GC()
		select {
		case <-isQuit:
			//释放资源

			//通过关闭这个客户端的channel,实现关闭本go程中创建的writeMsgToClient()子go
			close(clnt.ch)

			//delete map成员
			rwMutex.Lock()
			delete(onLineMap, clnt.addr)
			rwMutex.Unlock()

			//广播告知其他用户
			Message<- makeMsg(clnt, "logout")

			//结束本go程
			return

		case <-time.After(time.Second*30):
			//释放资源

			//通过关闭这个客户端的channel,实现关闭本go程中创建的writeMsgToClient()子go
			close(clnt.ch)

			//delete map成员
			rwMutex.Lock()
			delete(onLineMap, clnt.addr)
			rwMutex.Unlock()

			//广播告知其他用户
			Message<- makeMsg(clnt, "logout")

			//结束本go程
			return

		case <- isLive:
			//为了清空上一个case的计时

		}
	}
}

func makeMsg(clnt Client, str string) string {
	return "[" + clnt.addr + "]" + clnt.name + ":" + str
}

//main go结束，则其子go结束，因0--4g虚拟进程地址空间被释放；
//但是子go结束，子go中的子go（孙go）不会结束！！！
func writeMsgToClient(clnt Client, conn net.Conn) {
	for msg := range clnt.ch {
		conn.Write([]byte(msg+"\n"))
	}
}