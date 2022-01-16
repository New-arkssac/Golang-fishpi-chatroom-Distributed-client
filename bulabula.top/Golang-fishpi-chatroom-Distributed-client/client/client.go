package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
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
		conn.Write([]byte(recv))
	}
}

func main() {
	flag.Parse()
	host := fmt.Sprintf("%s:%s", ip, port)
	conn, err := net.Dial("tcp", host)
	if err != nil {
		log.Panicln("connect fail", err)
		return
	}
	defer conn.Close()
	go input(conn)
	for {
		var buf [1024]byte
		read := bufio.NewReader(conn)
		n, err := read.Read(buf[:])
		if err != nil {
			fmt.Println("recv failed, err:", err)
			return
		}
		fmt.Println(string(buf[:n]))
	}
}
