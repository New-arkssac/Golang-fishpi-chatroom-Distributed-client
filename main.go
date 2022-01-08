package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

var (
	host string
	port string
)

func init() {
	flag.StringVar(&host, "l", "0.0.0.0", "主机地址，默认0.0.0.0")
	flag.StringVar(&port, "p", "33333", "主机端口，默认33333")
}

func process(conn net.Conn) {
	go webSocketClient(conn)
	var apiKey string
	for apiKey == "" {
		apiKey = getUserNameAndPassword(conn)
		if apiKey != "" {
			log.Println(conn.RemoteAddr().String(), "loging SUCCESS")
			break
		}
		getMessage("apiKey nil, Please checking your username or password\n", conn)
	}
	getClient(conn, apiKey)
}

func webSocketClient(connect net.Conn) {
	client := websocket.Dialer{}
	conn, _, err := client.Dial("wss://fishpi.cn/chat-room-channel", nil)
	if err != nil {
		log.Println("link websocket error:", err)
		return
	}
	defer conn.Close()
	for {
		_, messageData, err := conn.ReadMessage()
		if err != nil {
			log.Println("Message get error", err)
		}
		m := make(map[string]string)
		json.Unmarshal(messageData, &m)
		userName, userNickName, userTime, userMsg := m["userName"], m["userNickname"], m["time"], m["md"]
		if userName != "" && userNickName != "" && userTime != "" && userMsg != "" {
			message := fmt.Sprintf("\n[%s]%s(%s):\n%s\n", userTime, userNickName, userName, userMsg)
			getMessage(message, connect)
		}
	}
}

func getUserNameAndPassword(conn net.Conn) string {
	for {
		var buf [1024]byte
		read := bufio.NewReader(conn)
		n, err := read.Read(buf[:])
		if err != nil {
			out := conn.RemoteAddr().String()
			log.Println(out, " Login out1", err)
		}
		recv := strings.Split(string(buf[:n]), "\n")[0]
		if strings.HasPrefix(recv, "{") && strings.HasSuffix(recv, "}") && strings.Contains(recv, "&&") {
			apiKey := verify(recv)
			return apiKey
		}
		_ = recv
	}
}

func getClient(conn net.Conn, apik string) {
	defer conn.Close()
	for {
		var buf [1024]byte
		read := bufio.NewReader(conn)
		n, err := read.Read(buf[:])
		if err != nil || n == 0 {
			out := conn.RemoteAddr().String()
			log.Println(out, " Login out")
			return
		}
		recv := strings.Split(string(buf[:n]), "\n")[0]
		go sendClientMessage(recv, apik)
		log.Println(conn.RemoteAddr().String(), recv)
	}
}

func getMessage(message string, conn net.Conn) {
	// defer conn.Close()
	conn.Write([]byte(message))
}

func verify(recv string) string {
	content := strings.TrimPrefix(recv, "{")
	content = strings.TrimSuffix(content, "}")
	arr := strings.Split(content, "&&")
	userName, passwd := arr[0], arr[len(arr)-1]
	apiKey := getApiKey(userName, passwd)
	_ = recv
	return apiKey
}

func getApiKey(userName string, passwd string) string {
	passwd = md5Hash(passwd)
	requestBody := fmt.Sprintf(`{"nameOrEmail": "%s", "userPassword": "%s"}`, userName, passwd)
	response, err := http.Post("https://fishpi.cn/api/getKey", "application/json", bytes.NewReader([]byte(requestBody)))
	if err != nil {
		log.Println("Get apiKey fail", err)
	}
	defer response.Body.Close()
	apiKey, _ := ioutil.ReadAll(response.Body)
	m := make(map[string]string)
	json.Unmarshal(apiKey, &m)
	if m["code"] == "-1" {
		log.Println(m["msg"])
	}

	return m["Key"]

}

func md5Hash(sum string) string {
	b := []byte(sum)
	m := md5.New()
	m.Write(b)
	hash := hex.EncodeToString(m.Sum(nil))
	return hash
}

func sendClientMessage(msg string, apiKey string) {
	if strings.HasPrefix(msg, "{") && strings.HasSuffix(msg, "}") && strings.Contains(msg, "&&") {
		return
	}
	requestBody := fmt.Sprintf(`{"apiKey": "%s", "content": "%s"}`, apiKey, msg)
	response, err := http.Post("https://fishpi.cn/chat-room/send", "application/json", bytes.NewReader([]byte(requestBody)))
	if err != nil {
		log.Println("Send Message error:", err)
	}
	defer response.Body.Close()
}

func main() {
	flag.Parse()
	localHost := fmt.Sprintf("%s:%s", host, port)
	listen, err := net.Listen("tcp", localHost)
	if err != nil {
		fmt.Println("Listen error:", err)
		return
	}
	for {
		connent, err := listen.Accept()
		log.Println(connent.RemoteAddr().String() + " content SUCCESS")
		if err != nil {
			fmt.Println("Accept error:", err)
			continue
		}
		go process(connent)
	}

}