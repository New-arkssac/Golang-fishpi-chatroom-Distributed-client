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
