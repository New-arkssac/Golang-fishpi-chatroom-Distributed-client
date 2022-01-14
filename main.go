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
	"time"

	"github.com/gorilla/websocket"
)

var ( // 程序参数设置
	host   string
	port   string
	client = &http.Client{}
	num    int
)

type info struct { //登录用户信息结构体
	ApiKey         string
	ConnectName    string
	RedRobotStatus bool
}

type chatRoom struct {
	Content      string `json:"content"`
	UserName     string `json:"userName"`
	UserNickName string `json:"userNickname"`
	UserMsg      string `json:"md"`
	Time         string `json:"time"`
	Oid          string `json:"oId"`
	Type         string `json:"type"`
}

type redInfo struct {
	Msg      string `json:"msg"`
	MsgType  string `json:"msgType"`
	Count    int    `json:"count"`
	Got      int    `json:"got"`
	Type     string `json:"type"`
	Recivers string `json:"recivers"`
}

type redOpenContent struct {
	Who []struct {
		GetMoney int    `json:"userMoney"`
		UserName string `json:"userName"`
	} `json:"who"` //红包数据结构体
}

type chatMore struct {
	Data []struct {
		Content string `json:"content"`
	} `json:"data"`
}

type heartBeat struct {
	MsgType string `json:"msgType"`
	Count   int    `json:"count"`
	Got     int    `json:"got"`
	Time    string `json:"time"`
	Who     []struct {
		UserMoney int `json:"userMoney"`
	} `json:"who"`
}

var status = make(map[string]info) // 缓存登录用户信息

var login string = `
##############################################
#请先登录: {your-nameOrEmail&&your-password}  #
##############################################
`

var help string = `
################################################
	{#redRobot} 开启红包机器人
################################################
`
var header = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/97.0.4692.71 Safari/537.36"

func init() {
	flag.StringVar(&host, "l", "0.0.0.0", "主机地址，默认0.0.0.0")
	flag.StringVar(&port, "p", "33333", "主机端口，默认33333")
}

func process(conn net.Conn) {
	var apiKey string
	var connectUserName string
	connectMessage(login, conn)
	go webSocketClient(conn) // 开启websocket会话
	defer conn.Close()       // 函数结束时关闭tcp连接
	for {                    // 接收tcp连接会话的输入
		var buf [1024]byte
		read := bufio.NewReader(conn)
		n, err := read.Read(buf[:])
		if err != nil {
			out := conn.RemoteAddr().String()
			delete(status, conn.RemoteAddr().String()) // tcp连接断开时删除对应的缓存
			log.Println(out, " Login out")
			fmt.Println(status)
			return
		}

		recv := strings.TrimSpace(string(buf[:n])) // 删除接收到的换行符
		fmt.Println(apiKey)
		fmt.Println(status)
		if strings.HasPrefix(recv, "{") && len(apiKey) == 32 && connectUserName != "" { // 检查是否是命令格式
			commandName, result := commandDealWicth(apiKey, connectUserName, recv, conn)
			if !result { // 检查命令
				message := fmt.Sprintf("%s执行失败\n", commandName)
				connectMessage(message, conn)
			}
			continue
		}

		if strings.HasPrefix(recv, "{") && strings.HasSuffix(recv, "}") && strings.Contains(recv, "&&") { //检查是否是登录命令
			content := strings.TrimPrefix(recv, "{")
			content = strings.TrimSuffix(content, "}")
			arr := strings.Split(content, "&&")
			userName, passwd := arr[0], arr[len(arr)-1]
			apiKey, connectUserName = getApiKey(userName, passwd, conn) // 获取apiKey
			status[conn.RemoteAddr().String()] = info{                  //使用连接ip与端口作为map键，存入info结构体
				ApiKey:         apiKey,          // 存入获取到的apiKey
				ConnectName:    connectUserName, // 存入apiKey的用户名
				RedRobotStatus: false,
			}
			continue
		}

		if apiKey == "" { // 检查是否拥有apiKey
			connectMessage(login, conn)
			continue
		}
		r := fmt.Sprintf("%s %s %s", conn.RemoteAddr().String(), connectUserName, recv)
		log.Println(r)                  // tcp会话输入历史记录
		sendClientMessage(recv, apiKey) // 发送用户输入的消息
	}
}

func commandDealWicth(apiKey string, connectUserName string, command string, conn net.Conn) (string, bool) { // 分发命令函数
	if command == "{#redRobot}" { // 开启红包机器人
		status[conn.RemoteAddr().String()] = info{
			ApiKey:         apiKey,          // 存入获取到的apiKey
			ConnectName:    connectUserName, // 存入apiKey的用户名
			RedRobotStatus: true,
		}
		connectMessage("\n红包机器人已开启\n", conn)
		return "redRbot", true
	}
	if command == "{#help}" { // help命令
		connectMessage(help, conn)
		return "help", true
	}
	return command, false
}

func webSocketClient(connect net.Conn) {
	client := websocket.Dialer{}
	conn, _, err := client.Dial("wss://fishpi.cn/chat-room-channel", nil) // 连接摸鱼派聊天室
	if err != nil {
		log.Println("link websocket error:", err)
		return
	}
	defer conn.Close() // 会话结束关闭连接
	for {
		_, messageData, err := conn.ReadMessage() // 获取信息
		if err != nil {
			log.Println("web linke err:", err)
			continue
		}
		var red redInfo
		var m chatRoom
		err1 := json.Unmarshal(messageData, &m)
		if err1 != nil { // 反序列化聊天室数据
			log.Println("Message get error1", err1)
		}
		if m.Content != "" && !strings.Contains(m.Content, "<") { // 检查红包数据
			err2 := json.Unmarshal([]byte(m.Content), &red)
			if err2 != nil {
				log.Println("red Message get error2", err2)
			}
		}
		go distribution(&red, &m, connect) // 分发数据
	}
}

func distribution(red *redInfo, m *chatRoom, conn net.Conn) {
	var user = status[conn.RemoteAddr().String()]
	if m.UserName != "" && m.UserMsg != "" { // 判断数据是否为空
		message := fmt.Sprintf("\n[%s]%s(%s):\n%s\n\n", m.Time, m.UserNickName, m.UserName, m.UserMsg)
		connectMessage(message, conn)
		return
	}
	if red.MsgType == "redPacket" && user.ApiKey != "" && user.ConnectName != "" { // 判断是否是红包信息
		message := fmt.Sprintf("\n[%s]%s(%s):\n%s\n", m.Time, m.UserNickName, m.UserName, red.Msg)
		go connectMessage(message, conn)
		go redPacketRobot(red.Type, red.Recivers, m.Oid, conn)
		return
	}
}

func redPacketRobot(typee string, recivers string, oId string, conn net.Conn) { // 红包机器人
	if !status[conn.RemoteAddr().String()].RedRobotStatus { //验证是否开启
		message := ("\n红包机器人: 你错过了一个红包!!!!!!!!!!\n")
		connectMessage(message, conn)
		return
	}
	name := status[conn.RemoteAddr().String()].ConnectName
	if typee == "heartbeat" {
		connectMessage("\n红包机器人: 发现心跳红包冲它!!\n", conn)
		moreContent(time.Now().Second(), oId, conn)
		return
	}
	if !strings.Contains(recivers, name) && recivers == "" || recivers == "[]" {
		connectMessage("\n红包机器人: 发现红包!开始出击!\n", conn)
		redRandomOrAverageOrMe(oId, conn)
	} else {
		connectMessage("\n红包机器人: 你的专属红包!\n", conn)
		redRandomOrAverageOrMe(oId, conn)
	}

}
func moreContent(statTime int, oId string, conn net.Conn) {
	var more chatMore
	var heart heartBeat
	request, err := http.NewRequest("GET", "https://fishpi.cn/chat-room/more?page=1", nil)
	if err != nil {
		fmt.Println(err)
	}
	request.Header.Set("User-Agent", header)
	response, err1 := client.Do(request)
	if err1 != nil {
		log.Println(err1)
	}
	defer response.Body.Close()
	r, _ := ioutil.ReadAll(response.Body)
	if err2 := json.Unmarshal(r, &more); err2 != nil {
		log.Println(err2)
	}
	if strings.Contains(more.Data[0].Content, "<") {
		moreContent(statTime, oId, conn)
		return
	}
	if err3 := json.Unmarshal([]byte(more.Data[0].Content), &heart); err3 != nil {
		log.Println(err3)
	}
	redHeartBeat(&heart, &more, statTime, oId, conn)
}

func redHeartBeat(heart *heartBeat, more *chatMore, statTime int, oId string, conn net.Conn) {
	if heart.Count == heart.Got {
		connectMessage("\n红包机器人: 红包已经没了，出手慢了呀!!\n", conn)
		return
	}
	if heart.Got == 0 || heart.Got != len(heart.Who) { // 判断是否有人领，没人领就继续递归
		moreContent(statTime, oId, conn)
		return
	}
	rush := 1 / (float64(heart.Count) - float64(heart.Got))
	for i := 0; i < heart.Got; i++ {
		if heart.Who[i].UserMoney > 0 {
			message := fmt.Sprintf("\n红包机器人: 已经被领了%d积分?超!这个红包不对劲!!快跑!!\n", heart.Who[num].UserMoney) //检查红包是否已经被人领取
			connectMessage(message, conn)
			return
		}
	}
	if rush > 0.5 || time.Now().Second()-statTime > 2 { // 递归两秒后退出
		connectMessage("\n红包机器人: 时间到了!!我忍不住了!!我冲了!!\n", conn)
		go redRandomOrAverageOrMe(oId, conn)
		return
	} else if heart.Who[heart.Got-1].UserMoney <= 0 {
		message := fmt.Sprintf("\n红包机器人: 稳住!!别急!!再等等!!成功率已经有%.2f%%了\n", rush*100)
		num++
		connectMessage(message, conn)
	}
	moreContent(statTime, oId, conn)
}

func redRandomOrAverageOrMe(oId string, conn net.Conn) {
	b := status[conn.RemoteAddr().String()]
	requestBody := fmt.Sprintf(`{"apiKey": "%s", "oId": "%s"}`, b.ApiKey, oId)
	request, err := http.NewRequest("POST", "https://fishpi.cn/chat-room/red-packet/open", bytes.NewReader([]byte(requestBody))) // 开启红包
	request.Header.Set("User-Agent", header)
	request.Header.Set("Content-Type", "application/json")
	if err != nil {
		log.Print("Get redPacket fail:", err)
	}
	r, err1 := client.Do(request)
	if err1 != nil {
		log.Println(err1)
	}
	defer r.Body.Close()
	response, _ := ioutil.ReadAll(r.Body)
	var m redOpenContent
	if err := json.Unmarshal(response, &m); err != nil {
		log.Println("redPacket err:", err)
	}
	for i := 0; i < len(m.Who); i++ { //检查是否打开红包
		if m.Who[i].UserName == b.ConnectName {
			if m.Who[i].GetMoney == 0 {
				mony := fmt.Sprintf("\n红包机器人: 呀哟，%d溢事件!!\n", m.Who[i].GetMoney)
				connectMessage(mony, conn)
				return
			}
			if m.Who[i].GetMoney < 0 {
				mony := fmt.Sprintf("\n红包机器人: 超!被反偷了%d积分!!!\n", m.Who[i].GetMoney)
				connectMessage(mony, conn)
				return
			}
			mony := fmt.Sprintf("\n红包机器人: 我帮你抢到了一个%d积分的红包!!!\n", m.Who[i].GetMoney)
			connectMessage(mony, conn)
			return
		}
	}
	connectMessage("\n红包机器人: 呀哟，没抢到!!一定是网络的问题!!!\n", conn)

}

func connectMessage(message string, conn net.Conn) { // 客户端接收数据
	conn.Write([]byte(message))
}

func getApiKey(userName string, passwd string, conn net.Conn) (string, string) { // 获取apiKey
	passwd = md5Hash(passwd)
	requestBody := fmt.Sprintf(`{"nameOrEmail": "%s", "userPassword": "%s"}`, userName, passwd)
	request, err := http.NewRequest("POST", "https://fishpi.cn/api/getKey", bytes.NewReader([]byte(requestBody)))
	request.Header.Set("User-Agent", header)
	request.Header.Set("Content-Type", "application/json")
	if err != nil {
		log.Println("Get apiKey fail:", err)
	}
	response, err1 := client.Do(request)
	if err1 != nil {
		log.Println("Get apiKey fail1:", err1)
	}
	defer response.Body.Close()
	apiKey, _ := ioutil.ReadAll(response.Body)
	m := make(map[string]interface{})
	json.Unmarshal(apiKey, &m)
	if m["code"].(float64) == -1 { // 判断是否获取成功
		msg := fmt.Sprintf("Login Message:%s\n", m["msg"].(string))
		connectMessage(msg, conn)
		return m["msg"].(string), userName
	}
	connectUserName := getUserInfo(m["Key"].(string), conn)
	msg := fmt.Sprintf("Login Message:%s(%s)\n%s\n", connectUserName, m["Key"].(string), "输入{#help}查看命令信息\n")
	log.Printf("%s %s Loging SUCCESS", conn.RemoteAddr().String(), connectUserName)
	connectMessage(msg, conn)
	return m["Key"].(string), connectUserName
}

func getUserInfo(apiKey string, conn net.Conn) string { // 获取用户信息
	type dataInfo struct {
		Data struct {
			UserName string `json:"userName"`
		} `json:"data"`
	}
	requestBody := fmt.Sprintf("https://fishpi.cn/api/user?apiKey=%s", apiKey)
	request, err := http.NewRequest("GET", requestBody, nil)
	request.Header.Set("User-Agent", header)
	if err != nil {
		log.Println("get connect User Info err:", err)
	}
	response, err1 := client.Do(request)
	if err1 != nil {
		log.Println(err1)
	}
	defer response.Body.Close()
	connectUserName, _ := ioutil.ReadAll(response.Body)
	var m dataInfo
	json.Unmarshal(connectUserName, &m)
	return m.Data.UserName
}

func md5Hash(sum string) string { // md5加密
	b := []byte(sum)
	m := md5.New()
	m.Write(b)
	hash := hex.EncodeToString(m.Sum(nil))
	return hash
}

func sendClientMessage(msg string, apiKey string) { // 发送客户端发送的数据
	if strings.HasPrefix(msg, "{") && strings.HasSuffix(msg, "}") && strings.Contains(msg, "&&") {
		return
	}
	requestBody := fmt.Sprintf(`{"apiKey": "%s", "content": "%s"}`, apiKey, msg)
	request, err := http.NewRequest("POST", "https://fishpi.cn/chat-room/send", bytes.NewReader([]byte(requestBody)))
	request.Header.Set("User-Agent", header)
	request.Header.Set("Content-Type", "application/json")
	if err != nil {
		log.Println("Send Message error:", err)
	}
	response, err1 := client.Do(request)
	if err1 != nil {
		log.Println(response)
	}
	defer response.Body.Close()
}

func main() { // 主函数
	flag.Parse()
	localHost := fmt.Sprintf("%s:%s", host, port)
	listen, err := net.Listen("tcp", localHost) // 建立tcp连接
	if err != nil {
		fmt.Println("Listen error:", err)
		return
	}
	for {
		connent, err := listen.Accept() //等待tcp连接
		log.Println(connent.RemoteAddr().String() + " Connect SUCCESS")
		if err != nil {
			fmt.Println("Accept error:", err)
			continue
		}
		go process(connent) // 创建tcp会话
	}

}
