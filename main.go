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
	ConnectMsg          []string
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
	status     = make(map[string]info) // 缓存登录用户信息
	header     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/97.0.4692.71 Safari/537.36"
	login      = "\n#请先登录: -yourNameOrEmail&&yourPassword #\n"
	help       = `
>   -help 查看帮助信息
>   -redrobot 开启红包机器人	//自动抢红包
>   -redinfo 查看红包信息
>   -timingtalk 定时说话	//-timingtalk:5 设置随机1-5分钟就自动发送直到活跃度是百分百就停止
>   -timingtalkm 查看定时说话列表
>   -sendred  发送红包	//-sendred-32-specify-1-bulabula 发送1个专属红包给bulabula，其他格式-sendred-32-random-10-
     >> type:random、heartbeat、specify、average
>   -nowactive	获取当前活跃度
>   -connectmsg	查看当前用户消息历史记录

`
)

func init() {
	flag.StringVar(&host, "l", "0.0.0.0", "主机地址，默认0.0.0.0")
	flag.StringVar(&port, "p", "33333", "主机端口，默认33333")
}

func process(id string, conn net.Conn) {
	var m = status[id]
	var ch = make(chan bool, 1)
	go func() {
		for i := range ch {
			if m.TimingTalk.ActivityStatus {
				return
			}
			if m.TimingTalk.TimingStatus && i {
				rand.Seed(time.Now().Unix())
				num := rand.Intn(m.TimingTalk.TalkMinit)
				time.Sleep(time.Duration(num+1) * time.Minute)
				if getStatus := getActivity(&m, conn); getStatus != 100 {
					rand.Seed(time.Now().Unix())
					num = rand.Intn(len(m.TimingTalk.TalkMessage))
					sendClientMessage(m.TimingTalk.TalkMessage[num], m.ApiKey, "", "", 0, 0) // 发送用户输入的消息
					ch <- true
				}
			}
		}
	}()
	go sendForClient(login, conn)
	// 定时发送消息函数
	go webSocketClient(&m, conn) // 开启websocket会话
	for {                        // 接收tcp连接会话的输入
		var buf [1024]byte
		read := bufio.NewReader(conn)
		n, err := read.Read(buf[:])
		if err != nil {
			out := conn.RemoteAddr().String()
			delete(status, id) // tcp连接断开时删除对应的缓存
			log.Println(out, " Login out")
			if closeErr := conn.Close(); closeErr != nil {
				log.Println(closeErr)
			}
			return
		}
		recv := strings.TrimSpace(string(buf[:n]))                                      // 删除接收到的换行符
		if strings.HasPrefix(recv, "-") && len(m.ApiKey) == 32 && m.ConnectName != "" { // 检查是否是命令格式
			commandName, result := commandDealWicth(&m, ch, recv, conn)
			if !result { // 检查命令
				message := fmt.Sprintf("\n%s执行失败\n", commandName)
				sendForClient(message, conn)
			}
			continue
		}

		if strings.HasPrefix(recv, "-") && strings.Contains(recv, "&&") { //检查是否是登录命令
			content := strings.TrimPrefix(recv, "-")
			arr := strings.Split(content, "&&")
			userName, passwd := arr[0], arr[len(arr)-1]
			m.ApiKey, m.ConnectName = getApiKey(userName, passwd, conn)
			continue
		}

		if m.ApiKey == "" { // 检查是否拥有apiKey
			sendForClient(login, conn)
			continue
		}
		r := fmt.Sprintf("%v-%v-%v %v:%v:%v %s %s %s", time.Now().Year(), time.Now().Month(), time.Now().Day(),
			time.Now().Hour(), time.Now().Minute(), time.Now().Second(), conn.RemoteAddr().String(), m.ConnectName, recv)
		m.ConnectMsg = append(m.ConnectMsg, r)
		sendClientMessage(recv, m.ApiKey, "", "", 0, 0) // 发送用户输入的消息
	}
}

func commandDealWicth(m *info, ch chan bool, command string, conn net.Conn) (string, bool) { // 分发命令函数
	commandMap := make(map[string]string)
	//  help
	commandMap["-help"] = help

	// redinfo
	commandMap["-redinfo"] = fmt.Sprintf("\n红包机器人:\n>用户名:%s\n>共抢了%d个红包\n>共获得%d积分\n>被反抢%d积分\n"+
		">错过红包%d个\n>总计收益%d\n",
		m.ConnectName, m.RedStatus.Find, m.RedStatus.GetPoint, m.RedStatus.OutPoint, m.RedStatus.MissRed,
		m.RedStatus.GetPoint+m.RedStatus.OutPoint)
	commandMap["-connectmsg"] = fmt.Sprintf("\n%s %v\n", m.ConnectName, m.ConnectMsg)
	// noactive
	if command == "-nowactive" {
		num := getActivity(m, conn)
		commandMap["-nowactive"] = fmt.Sprintf("\n当前%s用户的活跃度是%f\n", m.ConnectName, num)
	}

	// timingtalkm
	if len(m.TimingTalk.TalkMessage) == 0 {
		m.TimingTalk.TalkMessage = append(m.TimingTalk.TalkMessage, "小冰 说个笑话")
		m.TimingTalk.TalkMessage = append(m.TimingTalk.TalkMessage, "小冰 lsp排行榜")
		m.TimingTalk.TalkMessage = append(m.TimingTalk.TalkMessage, "小冰 你人呢？")
		m.TimingTalk.TalkMessage = append(m.TimingTalk.TalkMessage, "小冰 去打劫")
		m.TimingTalk.TalkMessage = append(m.TimingTalk.TalkMessage, "摸鱼办第一纪律委提醒您：\n聊天千万条，友善第一条;"+
			"\n灌水不规范，扣分两行泪。\n我正在认真巡逻中，不要被我逮到哦～![doge](https://cdn.jsdelivr.net/npm/vditor/dist/images/"+
			"emoji/doge.png)\n详细社区守则请看：[摸鱼守则](https://fishpi.cn/article/1631779202219)")
	}
	commandMap["-timingtalkm"] = fmt.Sprintln(m.TimingTalk.TalkMessage)

	// redRobot
	if command == "-redrobot" && m.RedRobotStatus {
		commandMap["-redrobot"] = "\n红包机器人已关闭\n\n"
		m.RedRobotStatus = false
	} else if command == "-redrobot" && !m.RedRobotStatus {
		commandMap["-redrobot"] = "\n红包机器人已开启\n\n"
		m.RedRobotStatus = true
	}

	//timingTalk
	if resul, _ := regexp.MatchString(`^-timingtalk:\d+$`, command); resul && m.TimingTalk.ActivityStatus {
		commandMap[command] = "\n活跃度已满请不要再开启定时说话模式\n\n"
	} else if command == "-timingtalk" {
		commandMap[command] = "\n定时说话模式已关闭\n\n"
		m.TimingTalk.TalkMinit = 0
		m.TimingTalk.TimingStatus = false
	} else if resul, _ := regexp.MatchString(`^-timingtalk:\d+$`, command); resul {
		out1 := regexp.MustCompile(`\d+`).FindString(command)
		i, _ := strconv.ParseInt(out1, 10, 0)
		if i < 5 || i > 20 {
			commandMap[command] = "\n定时说话模式已失败,定时时间不允许小于5分钟大于20分钟\n\n"
		} else {
			commandMap[command] = "\n定时说话模式已开启\n\n"
			m.TimingTalk.TalkMinit = int(i)
			m.TimingTalk.TimingStatus = true
			ch <- true
		}
	}

	// sendred
	if resul, _ := regexp.MatchString(`^-sendred-\d+-(random|heartbeat|specify|average)-`, command); resul {
		out1 := regexp.MustCompile(`(random|heartbeat|specify|average)`).FindString(command)
		out2 := regexp.MustCompile(`\d+`).FindAllStringSubmatch(command, 2)
		out3 := regexp.MustCompile(`\w+$`).FindString(command)
		i, _ := strconv.ParseInt(out2[0][0], 10, 0)
		j, _ := strconv.ParseInt(out2[1][0], 10, 0)
		if i < 32 {
			commandMap[command] = "\n不允许发送小于32积分的红包\n\n"
		} else {
			commandMap[command] = fmt.Sprintf("\n开始发送%s红包\n", out1)
			typeMap := map[string]string{
				"random":    "摸鱼着，事竟成！",
				"heartbeat": "玩的就是心跳！",
				"average":   "平分红包，人人有份！",
				"specify":   "试试看，这是给你的红包吗？",
			}
			go sendClientMessage(typeMap[out1], m.ApiKey, out1, out3, j, i)
		}
	}

	if commandMap[command] == "" {
		return command, false
	}

	sendForClient(commandMap[command], conn)
	return command, true
}

func webSocketClient(b *info, connect net.Conn) {
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
		go distribution(b, &red, &m, connect) // 分发数据
	}
}

func distribution(b *info, red *redInfo, m *chatRoom, conn net.Conn) {
	if m.UserName != "" && m.UserMsg != "" { // 判断数据是否为空
		message := fmt.Sprintf("\n[%s]%s(%s):\n%s\n\n", m.Time, m.UserNickName, m.UserName, m.UserMsg)
		sendForClient(message, conn)
		return
	}
	if red.MsgType == "redPacket" { // 判断是否是红包信息
		message := fmt.Sprintf("\n[%s]%s(%s):\n红包！！！！！(%s)\n\n", m.Time, m.UserNickName, m.UserName, red.Msg)
		sendForClient(message, conn)
		redPacketRobot(b, red.Type, red.Recivers, m.Oid, conn)
		return
	}
}

func getActivity(m *info, conn net.Conn) float64 {
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
		message := fmt.Sprintf("\n%s活跃度已满%.2f%%!\n", m.ConnectName, b.Liveness)
		sendForClient(message, conn)
		m.TimingTalk.TimingStatus = false
		m.TimingTalk.ActivityStatus = true
		return b.Liveness
	}
	return b.Liveness
}

func redPacketRobot(m *info, typee, recivers string, oId string, conn net.Conn) { // 红包机器人
	if !m.RedRobotStatus {
		message := "\n红包机器人: 你错过了一个红包!!!!!!!!!!\n"
		sendForClient(message, conn)
		m.RedStatus.MissRed++
		return
	}
	m.RedStatus.Find++
	if typee == "heartbeat" {
		sendForClient("\n红包机器人: 发现心跳红包冲它!!\n", conn)
		moreContent(time.Now().Second(), m, oId, conn)
		return
	}
	if !strings.Contains(recivers, m.ConnectName) && recivers == "" || recivers == "[]" {
		sendForClient("\n红包机器人: 发现红包!开始出击!\n", conn)
		redRandomOrAverageOrMe(m, oId, conn)
	} else if strings.Contains(recivers, m.ConnectName) {
		sendForClient("\n红包机器人: 你的专属红包!\n", conn)
		redRandomOrAverageOrMe(m, oId, conn)
	}

}
func moreContent(nowTime int, m *info, oId string, conn net.Conn) { // 获取领取信息
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
		moreContent(nowTime, m, oId, conn)
		return
	}
	if err3 := json.Unmarshal([]byte(more.Data[0].Content), &heart); err3 != nil {
		log.Println(err3)
	}
	redHeartBeat(&heart, m, nowTime, oId, conn)
}

func redHeartBeat(heart *heartBeat, m *info, nowTime int, oId string, conn net.Conn) {
	if heart.Count == heart.Got {
		sendForClient("\n红包机器人: 红包已经没了，出手慢了呀!!\n", conn)
		return
	}
	if heart.Got == 0 || heart.Got != len(heart.Who) { // 判断是否有人领，没人领就继续递归
		moreContent(nowTime, m, oId, conn)
		return
	}
	rush := 1 / (float64(heart.Count) - float64(heart.Got))
	for i := 0; i < heart.Got; i++ {
		if heart.Who[i].UserMoney > 0 {
			message := fmt.Sprintf("\n红包机器人: 已经被领了%d积分?超!这个红包不对劲!!快跑!!\n", heart.Who[i].UserMoney) //检查红包是否已经被人领取
			sendForClient(message, conn)
			return
		}
	}
	if rush > 0.5 || time.Now().Second()-nowTime > 3 || heart.Count-heart.Got == 1 { // 递归两秒后退出
		sendForClient("\n红包机器人: 时间到了!!我忍不住了!!我冲了!!\n", conn)
		go redRandomOrAverageOrMe(m, oId, conn)
		return
	} else {
		message := fmt.Sprintf("\n红包机器人: 稳住!!别急!!再等等!!成功率已经有%.2f%%了\n", rush*float64(heart.Count))
		sendForClient(message, conn)
		moreContent(nowTime, m, oId, conn)
	}
	return
}

func redRandomOrAverageOrMe(b *info, oId string, conn net.Conn) {
	time.Sleep(2 * time.Second)
	requestBody := fmt.Sprintf(`{"apiKey": "%s", "oId": "%s"}`, b.ApiKey, oId)
	request, err := http.NewRequest("POST", "https://fishpi.cn/chat-room/red-packet/open",
		bytes.NewReader([]byte(requestBody))) // 开启红包
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
				sendForClient(money, conn)
				return
			}
			if m.Who[i].GetMoney < 0 {
				money := fmt.Sprintf("\n红包机器人: 超!被反偷了%d积分!!!\n", m.Who[i].GetMoney)
				sendForClient(money, conn)
				b.RedStatus.OutPoint += m.Who[i].GetMoney
				return
			}
			money := fmt.Sprintf("\n红包机器人: 我帮你抢到了一个%d积分的红包!!!\n", m.Who[i].GetMoney)
			sendForClient(money, conn)
			b.RedStatus.GetPoint += m.Who[i].GetMoney
			return
		}
	}
	sendForClient("\n红包机器人: 呀哟，没抢到!!一定是网络的问题!!!\n", conn)
	b.RedStatus.MissRed++

}

func getApiKey(userName string, passwd string, conn net.Conn) (string, string) { // 获取apiKey
	passwd = md5Hash(passwd)
	requestBody := fmt.Sprintf(`{"nameOrEmail": "%s", "userPassword": "%s"}`, userName, passwd)
	request, err := http.NewRequest("POST", "https://fishpi.cn/api/getKey",
		bytes.NewReader([]byte(requestBody)))
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
		sendForClient(msg, conn)
		return m["msg"].(string), userName
	}
	connectUserName := getUserInfo(m["Key"].(string))
	msg := fmt.Sprintf("Login Message:%s(%s)\n%s\n", connectUserName, m["Key"].(string), "输入-help查看命令信息\n")
	log.Printf("%s %s Loging SUCCESS", conn.RemoteAddr().String(), connectUserName)
	sendForClient(msg, conn)
	return m["Key"].(string), connectUserName
}

func sendForClient(msg string, conn net.Conn) {
	_, _ = conn.Write([]byte(msg))
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

func sendClientMessage(msg, apiKey, typee, name string, count, money int64) { // 发送客户端发送的数据
	if strings.HasPrefix(msg, "-") && strings.Contains(msg, "&&") {
		return
	}
	requestBody := fmt.Sprintf(`{"apiKey": "%s", "content": "%s"}`, apiKey, msg)
	if money != 0 {
		requestBody = fmt.Sprintf(`{"apiKey": "%s", "content":"[redpacket]{\"type\":\"%s\",\"money\":\"%v\",\"count\":\"%v\",\"msg\":\"%s\",\"recivers\":[\"%s\"]}[/redpacket]"}`, apiKey, typee, money, count, msg, name)
	}
	request, err := http.NewRequest("POST", "https://fishpi.cn/chat-room/send",
		bytes.NewReader([]byte(requestBody)))
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
		id := md5Hash(time.Now().String())
		log.Println(connent.RemoteAddr().String() + " Connect SUCCESS")
		if err != nil {
			log.Println("Accept error:", err)
			continue
		}
		go process(id, connent) // 创建tcp会话
	}

}
