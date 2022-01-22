# Golang-fishpi-chatroom-Distributed-client

**Gayhub地址:**

[摸鱼派Golang分布式客户端](https://github.com/New-arkssac/Golang-fishpi-chatroom-Distributed-client)
>golang第三方库
>
>https://github.com/gorilla/websocket

---
## 更新日志
>2022/1/14 &nbsp;&nbsp;&nbsp;&nbsp; 更新了红包机器人 可以自动抢红包，提高抢心跳红包成功的概率
> 
>2022/1/16  &nbsp;&nbsp;&nbsp;&nbsp; 更新了定时发消息的功能
> 
>2022/1/20 &nbsp;&nbsp;&nbsp;&nbsp; 更新了发送红包，当前活跃度，查看历史发送消息记录，查看定时说话列表功能
> 
> 2022/1/22 &nbsp;&nbsp;&nbsp;&nbsp; 更新了获取昨日奖励



## 目前功能

* 接发消息
* 红包机器人
* 定时发消息
* 发送红包
* 查看历史消息
* 查看活跃度
* 查看定时说话列表
* 获取昨日奖励

> 因为上班摸鱼学的golang，边学边写的，所以暂时仅支持这些功能😋 )


## 创作理念（其实就是突发奇想）

一天布某人在认真(**摸鱼**)上班工作的时候，看着电脑面前的甲方服务器终端，发呆----

突然有一种想法，“可恶啊！我好想摸鱼！好想在摸鱼派的聊天室里水活跃度啊！！！”

可是布某人左右逢敌，左边是甲方爸爸，右边是顶头上司

布某人怎敢冒着巨大风险打开浏览器盯着摸鱼派的聊天室呢？

这时布某人灵光一闪！！“我来写一个又隐蔽又轻松又能在终端里接发消息的客户端吧！”

他这样想到。

---

### 条件-> 轻松又隐蔽

既然要隐蔽那就不能在甲方服务器上安装和创建任何东西了

> 不然因为摸鱼丢了工作这就得不偿失了嘛😋

> 解决办法：
>
> 直接使用服务器上的环境，摸鱼派聊天室的api数据从我的机器上面拿，服务器只接收临时数据
>
> ~~条件-> 轻松又隐蔽~~  解决~

## 分布式客户端诞生

`Golang-fishpi-chatroom-Distributed-client`缩写`GDC`

用Golang写了一个websocket的客户端，然后再用socket起一个服务端并且把从摸鱼派聊天室接收到的json数据进行处理然后分发给对`GDC`进行**tcp连接的客户端**。

重点**TCP客户端**，只要是能进行tcp连接的客户端就可以进行接发消息

> 获取用户apiKey格式
>
> {your-username&&your-password}

可以多用户在线，放内网里，开启一个服务多人连接一起嗨皮，一起摸鱼

> 也可以放公网vps，自建服务器上，但是目前暂不支持消息加密服务，所以不建议放公网上


### 例子

**python的tcp客户端**

```python
#!/bin/python3
# -*- coding:utf-8 -*-
import socket
import threading
import sys

address = sys.argv[1]  # 服务端地址
port = int(sys.argv[2])  # 端口


def link():
    sc = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sc.connect((address, port))
    threading.Thread(target=send, args=(sc, )).start()
    while True:
        try:
            a = sc.recv(1024)
            print(a.decode("utf-8"))
        except Exception:  # ctrl c退出
            sc.close()
            return


def send(sc):
    while True:
        msg = input("")
        if msg == "{quit}":
            break
        sc.sendall(msg.encode())


c = threading.Thread(target=link())
c.setDaemon(True)
c.start()
```

![image.png](https://pwl.stackoverflow.wiki/2022/01/image-1e7fe38f.png)

**Golang的tcp客户端**

```go
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

var ip string
var port string

func init() {
	flag.StringVar(&ip, "i", "", "服务ip地址")
	flag.StringVar(&port, "p", "33333", "服务端口，默认33333")
}

func input(conn net.Conn) {
	for {
		var buf [1024]byte
		read := bufio.NewReader(os.Stdin)
		m, err := read.Read(buf[:])
		if err != nil {
			log.Println("Login out")
			return
		}
		recv := strings.Split(string(buf[:m]), "\n")[0]
		if num, writeErr := conn.Write([]byte(recv)); writeErr != nil {
			log.Printf("写入失败%d次,err:%s", num, writeErr)
		}
	}
}

func main() {
	flag.Parse()
	host := fmt.Sprintf("%s:%s", ip, port)
	conn, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		log.Panicln("connect fail", err)
		return
	}
	go input(conn)
	for {
		var buf [1024]byte
		read := bufio.NewReader(conn)
		n, err := read.Read(buf[:])
		if err != nil && n == 0 {
			fmt.Println("recv failed, err:", err)
			if closeErr := conn.Close(); closeErr != nil {
				log.Println("关闭失败:", closeErr)
			}
			return
		}
		fmt.Println(string(buf[:]))
	}
}
```

![image.png](https://pwl.stackoverflow.wiki/2022/01/image-c6aea66a.png)

**甚至是netcat**

![image.png](https://pwl.stackoverflow.wiki/2022/01/image-72f882bb.png)
