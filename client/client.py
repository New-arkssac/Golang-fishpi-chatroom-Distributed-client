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
        except KeyboardInterrupt:  # ctrl c退出
            sc.close()
            return


def send(sc):
    while True:
        msg = input(">>")
        sc.sendall(msg.encode())


c = threading.Thread(target=link())
c.setDaemon(True)
c.start()
