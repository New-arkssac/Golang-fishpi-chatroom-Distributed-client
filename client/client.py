#!/bin/python3
# -*- coding:utf-8 -*-
import socket
from _thread import start_new_thread
import sys

address = sys.argv[1]
port = int(sys.argv[2])

def link():
    sc = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sc.connect((address, port))
    start_new_thread(send, (sc,))
    while True:
        try:
           a = sc.recv(1024)
           print(a.decode("utf-8"))
        except KeyboardInterrupt: # ctrl c退出
            sc.close()
            return

def send(sc):
    while True:
        msg = input("")
        if msg == "{quit}":
            sys.exit(0)
        sc.sendall(msg.encode())

link()

