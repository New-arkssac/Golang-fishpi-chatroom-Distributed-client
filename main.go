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
	"math/rand"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type info struct { //登录用户信息结构体
	ApiKey, ConnectName string
	RedRobotStatus      bool
	TimingTalk          struct {
		TimingStatus, ActivityStatus bool
		TalkMessage                  []string
		TalkMinit                    int
	}
	RedStatus struct {
		Find, GetPoint, OutPoint, MissRed int
	}
}

type chatRoom struct {
	Content      string `json:"content"`
	UserName     string `json:"userName"`
	UserNickName string `json:"userNickname"`
	UserMsg      string `json:"md"`
	Time         string `json:"time"`
	Oid          string `json:"oId"`
	Type         string `json:"type"` // 聊天室信息结构体
}

type redInfo struct {
	Msg      string `json:"msg"`
	MsgType  string `json:"msgType"`
	Count    int    `json:"count"`
	Got      int    `json:"got"`
	Type     string `json:"type"`
	Recivers string `json:"recivers"` // 红包数据结构体
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
	} `json:"data"` // 领取信息结构体
}

type heartBeat struct {
	MsgType string `json:"msgType"`
	Count   int    `json:"count"`
	Got     int    `json:"got"`
	Time    string `json:"time"`
	Who     []struct {
		UserMoney int `json:"userMoney"`
	} `json:"who"` // 领取信息列表结构体
}

var ( // 程序参数设置
	host, port string
	client     = &http.Client{}
	status     = make(map[string]*info) // 缓存登录用户信息
	header     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/97.0.4692.71 Safari/537.36"
	login      = "\n#请先登录: -yourNameOrEmail&&yourPassword #\n"
	help       = `
>   -help 查看帮助信息
>   -redrobot 开启红包机器人	//自动抢红包
>   -redinfo 查看红包信息
>   -timingtalk 定时说话	//-timingtalk:5 小冰 说个笑话 设置每隔五分钟就自动发消息直到活跃度是百分百就停止

`
)

func init() {
	flag.StringVar(&host, "l", "0.0.0.0", "主机地址，默认0.0.0.0")
	flag.StringVar(&port, "p", "33333", "主机端口，默认33333")
}

func process(conn net.Conn) {
	status[conn.RemoteAddr().String()] = &info{ // 初始化连接用户信息
		ApiKey:         "",
		ConnectName:    "",
		RedRobotStatus: false,
		TimingTalk: struct {
			TimingStatus, ActivityStatus bool
			TalkMessage                  []string
			TalkMinit                    int
		}{ActivityStatus: false, TalkMessage: []string{}, TalkMinit: 5},
		RedStatus: struct {
			Find, GetPoint, OutPoint, MissRed int
		}{Find: 0, GetPoint: 0, OutPoint: 0, MissRed: 0},
	}

	var m = status[conn.RemoteAddr().String()]
	var ch = make(chan bool, 1)
	go func() {
		for i := range ch {
			if m == nil {
				return
			}
			time.Sleep(time.Duration(m.TimingTalk.TalkMinit) * time.Minute)
			if m.TimingTalk.ActivityStatus {
				return
			}
			if m.TimingTalk.TimingStatus && i {
				go getActivity(ch, conn)
			}
		}
	}()
	connectMessage(login, conn)
	// 定时发送消息函数
	go webSocketClient(conn) // 开启websocket会话
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			log.Println(closeErr)
		}
	}()   // 函数结束时关闭tcp连接
	for { // 接收tcp连接会话的输入
		var buf [1024]byte
		read := bufio.NewReader(conn)
		n, err := read.Read(buf[:])
		if err != nil {
			out := conn.RemoteAddr().String()
			delete(status, conn.RemoteAddr().String()) // tcp连接断开时删除对应的缓存
			log.Println(out, " Login out")
			return
		}
		recv := strings.TrimSpace(string(buf[:n]))                                            // 删除接收到的换行符
		if strings.HasPrefix(recv, "-") && len((*m).ApiKey) == 32 && (*m).ConnectName != "" { // 检查是否是命令格式
			commandName, result := commandDealWicth(ch, recv, conn)
			if !result { // 检查命令
				message := fmt.Sprintf("\n%s执行失败\n", commandName)
				connectMessage(message, conn)
			}
			continue
		}

		if strings.HasPrefix(recv, "-") && strings.Contains(recv, "&&") { //检查是否是登录命令
			content := strings.TrimPrefix(recv, "-")
			arr := strings.Split(content, "&&")
			userName, passwd := arr[0], arr[len(arr)-1]
			(*m).ApiKey, (*m).ConnectName = getApiKey(userName, passwd, conn)
			continue
		}

		if (*m).ApiKey == "" { // 检查是否拥有apiKey
			connectMessage(login, conn)
			continue
		}
		r := fmt.Sprintf("%s %s %s", conn.RemoteAddr().String(), (*m).ConnectName, recv)
		log.Println(r)                       // tcp会话输入历史记录
		sendClientMessage(recv, (*m).ApiKey) // 发送用户输入的消息
	}
}

func commandDealWicth(ch chan bool, command string, conn net.Conn) (string, bool) { // 分发命令函数
	var m = status[conn.RemoteAddr().String()]
	commandMap := make(map[string]string)
	//  help
	commandMap["-help"] = help
	// redinfo
	commandMap["-redinfo"] = fmt.Sprintf("\n红包机器人:\n>用户名:%s\n>共抢了%d个红包\n>共获得%d积分\n>被反抢%d积分\n"+
		">错过红包%d个\n>总计收益%d\n",
		(*m).ConnectName, (*m).RedStatus.Find, (*m).RedStatus.GetPoint, (*m).RedStatus.OutPoint, (*m).RedStatus.MissRed,
		(*m).RedStatus.GetPoint+(*m).RedStatus.OutPoint)
	// redRobot
	if command == "-redrobot" && (*m).RedRobotStatus {
		commandMap["-redrobot"] = "\n红包机器人已关闭\n\n"
		(*m).RedRobotStatus = false
	} else if command == "-redrobot" && !(*m).RedRobotStatus {
		commandMap["-redrobot"] = "\n红包机器人已开启\n\n"
		(*m).RedRobotStatus = true
	}
	//timingTalk
	if resul, _ := regexp.MatchString(`^-timingtalk:\d+\s.*$`, command); resul && m.TimingTalk.ActivityStatus {
		commandMap[command] = "\n活跃度已满请不要再开启定时说话模式\n\n"
	} else if command == "-timingtalk" {
		commandMap[command] = "\n定时说话模式已关闭\n\n"
		(*m).TimingTalk.TalkMinit = 0
		(*m).TimingTalk.TimingStatus = false
	} else if resul, _ := regexp.MatchString(`^-timingtalk:\d+\s.*$`, command); resul {
		out1 := regexp.MustCompile(`\d+`).FindStringSubmatch(command)
		out2 := regexp.MustCompile(`\s.*$`).FindStringSubmatch(command)
		i, _ := strconv.ParseInt(out1[0], 10, 0)
		if i < 5 {
			commandMap[command] = "\n定时说话模式已失败,定时时间不允许小于5分钟\n\n"
			return command, false
		}
		commandMap[command] = "\n定时说话模式已开启\n\n"
		(*m).TimingTalk.TalkMessage = append((*m).TimingTalk.TalkMessage, out2[0])
		(*m).TimingTalk.TalkMinit = int(i)
		(*m).TimingTalk.TimingStatus = true
		ch <- true
	}

	if commandMap[command] == "" {
		return command, false
	}

	connectMessage(commandMap[command], conn)
	return command, true
}

func webSocketClient(connect net.Conn) {
	client := websocket.Dialer{}
	conn, _, err := client.Dial("wss://fishpi.cn/chat-room-channel", nil) // 连接摸鱼派聊天室
	if err != nil {
		log.Println("link websocket error:", err)
		return
	}

	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			log.Println(closeErr)
		}
	}() // 会话结束关闭连接

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
		message := fmt.Sprintf("\n[%s]%s(%s):\n红包(%s)\n", m.Time, m.UserNickName, m.UserName, red.Msg)
		go connectMessage(message, conn)
		go redPacketRobot(red.Type, red.Recivers, m.Oid, conn)
		return
	}
}

func getActivity(ch chan bool, conn net.Conn) {
	m := status[conn.RemoteAddr().String()]
	if m == nil {
		return
	}
	type activity struct {
		Liveness float64 `json:"liveness"`
	}
	url := fmt.Sprintf("https://fishpi.cn/user/liveness?apiKey=%s", m.ApiKey)
	request, err := http.NewRequest("GET", url, nil)
	request.Header.Set("User-Agent", header)
	if err != nil {
		log.Println("设置活跃度失败请求:", err)
	}
	r, err1 := client.Do(request)
	defer func() {
		if err1 != nil {
			log.Println("活跃度获取失败:", err1)
		}
	}()
	var b activity
	response, _ := ioutil.ReadAll(r.Body)
	if err3 := json.Unmarshal(response, &b); err3 != nil {
		log.Println("活跃度json转码失败:", err3)
	}
	if b.Liveness == 100.00 {
		message := fmt.Sprintf("\n%s活跃度已满%2f%%!\n", m.ConnectName, b.Liveness)
		connectMessage(message, conn)
		(*m).TimingTalk.TimingStatus = false
		(*m).TimingTalk.ActivityStatus = true
		return
	}
	rand.Seed(time.Now().Unix())
	num := rand.Intn(len(m.TimingTalk.TalkMessage))
	sendClientMessage(m.TimingTalk.TalkMessage[num], m.ApiKey)
	ch <- true
}

func redPacketRobot(typee, recivers string, oId string, conn net.Conn) { // 红包机器人
	if !status[conn.RemoteAddr().String()].RedRobotStatus {              //验证是否开启
		message := "\n红包机器人: 你错过了一个红包!!!!!!!!!!\n"
		connectMessage(message, conn)
		return
	}

	m := status[conn.RemoteAddr().String()]
	(*m).RedStatus.Find++
	if typee == "heartbeat" {
		connectMessage("\n红包机器人: 发现心跳红包冲它!!\n", conn)
		moreContent(time.Now().Second(), oId, conn)
		return
	}
	if !strings.Contains(recivers, m.ConnectName) && recivers == "" || recivers == "[]" {
		connectMessage("\n红包机器人: 发现红包!开始出击!\n", conn)
		redRandomOrAverageOrMe(oId, conn)
	} else {
		connectMessage("\n红包机器人: 你的专属红包!\n", conn)
		redRandomOrAverageOrMe(oId, conn)
	}

}
func moreContent(statTime int, oId string, conn net.Conn) { // 获取领取信息
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
	redHeartBeat(&heart, statTime, oId, conn)
}

func redHeartBeat(heart *heartBeat, statTime int, oId string, conn net.Conn) {
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
			message := fmt.Sprintf("\n红包机器人: 已经被领了%d积分?超!这个红包不对劲!!快跑!!\n", heart.Who[i].UserMoney) //检查红包是否已经被人领取
			connectMessage(message, conn)
			return
		}
	}
	if rush > 0.5 || time.Now().Second()-statTime > 2 || heart.Count-heart.Got == 1 { // 递归两秒后退出
		connectMessage("\n红包机器人: 时间到了!!我忍不住了!!我冲了!!\n", conn)
		go redRandomOrAverageOrMe(oId, conn)
		return
	} else {
		message := fmt.Sprintf("\n红包机器人: 稳住!!别急!!再等等!!成功率已经有%f%%了\n", rush*100)
		connectMessage(message, conn)
		moreContent(statTime, oId, conn)
		return
	}
}

func redRandomOrAverageOrMe(oId string, conn net.Conn) {
	b := status[conn.RemoteAddr().String()]
	fmt.Println(b)
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
	defer func() {
		if closeErr := r.Body.Close(); closeErr != nil {
			log.Println(closeErr)
		}
	}()
	response, _ := ioutil.ReadAll(r.Body)
	var m redOpenContent
	if err := json.Unmarshal(response, &m); err != nil {
		log.Println("redPacket err:", err)
	}
	for i := 0; i < len(m.Who); i++ { //检查是否打开红包
		if m.Who[i].UserName == b.ConnectName {
			if m.Who[i].GetMoney == 0 {
				money := fmt.Sprintf("\n红包机器人: 呀哟，%d溢事件!!\n", m.Who[i].GetMoney)
				connectMessage(money, conn)
				return
			}
			if m.Who[i].GetMoney < 0 {
				money := fmt.Sprintf("\n红包机器人: 超!被反偷了%d积分!!!\n", m.Who[i].GetMoney)
				connectMessage(money, conn)
				(*b).RedStatus.OutPoint += m.Who[i].GetMoney
				return
			}
			money := fmt.Sprintf("\n红包机器人: 我帮你抢到了一个%d积分的红包!!!\n", m.Who[i].GetMoney)
			connectMessage(money, conn)
			(*b).RedStatus.GetPoint += m.Who[i].GetMoney
			return
		}
	}
	connectMessage("\n红包机器人: 呀哟，没抢到!!一定是网络的问题!!!\n", conn)
	(*b).RedStatus.MissRed++

}

func connectMessage(message string, conn net.Conn) { // 客户端接收数据
	if i, err := conn.Write([]byte(message)); err != nil {
		log.Println("connectMessage err:", err, i)
	}
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
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			log.Println(closeErr)
		}
	}()
	apiKey, _ := ioutil.ReadAll(response.Body)
	m := make(map[string]interface{})
	if err1 := json.Unmarshal(apiKey, &m); err1 != nil {
		log.Println(err1)
	}
	if m["code"].(float64) == -1 { // 判断是否获取成功
		msg := fmt.Sprintf("Login Message:%s\n", m["msg"].(string))
		connectMessage(msg, conn)
		return m["msg"].(string), userName
	}
	connectUserName := getUserInfo(m["Key"].(string))
	msg := fmt.Sprintf("Login Message:%s(%s)\n%s\n", connectUserName, m["Key"].(string), "输入-help查看命令信息\n")
	log.Printf("%s %s Loging SUCCESS", conn.RemoteAddr().String(), connectUserName)
	connectMessage(msg, conn)
	return m["Key"].(string), connectUserName
}

func getUserInfo(apiKey string) string { // 获取用户信息
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

	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			log.Println(closeErr)
		}
	}()
	connectUserName, _ := ioutil.ReadAll(response.Body)
	var m dataInfo
	if err1 := json.Unmarshal(connectUserName, &m); err1 != nil {
		log.Println(err1)
	}
	return m.Data.UserName
}

func md5Hash(sum string) string { // md5加密
	b := []byte(sum)
	m := md5.New()
	m.Write(b)
	hash := hex.EncodeToString(m.Sum(nil))
	return hash
}

func sendClientMessage(msg, apiKey string) { // 发送客户端发送的数据
	if strings.HasPrefix(msg, "-") && strings.Contains(msg, "&&") {
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
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			log.Println("发送消息关闭失败", closeErr)
		}
	}()
}

func main() { // 主函数
	flag.Parse()
	localHost := fmt.Sprintf("%s:%s", host, port)
	listen, err := net.Listen("tcp", localHost) // 建立tcp连接
	log.Printf("开始监听%s", port)
	if err != nil {
		log.Println("监听失败:", err)
		return
	}
	for {
		connent, err := listen.Accept() //等待tcp连接
		log.Println(connent.RemoteAddr().String() + " Connect SUCCESS")
		if err != nil {
			log.Println("Accept error:", err)
			continue
		}
		go process(connent) // 创建tcp会话
	}

}
