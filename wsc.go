package main

import (
	"PortForwardGo/zlog"
	"net"
	"time"
	"golang.org/x/net/websocket"
)

func LoadWSCRules(i string){
	Setting.mu.Lock()
	tcpaddress, _ := net.ResolveTCPAddr("tcp", ":"+Setting.Config.Rules[i].Listen)
	
	ln,err := net.ListenTCP("tcp",tcpaddress)
	if err == nil {
		zlog.Info("Loaded [",i,"] (WebSocket Client) ", Setting.Config.Rules[i].Listen, " => ", Setting.Config.Rules[i].Forward)
	}else{
		zlog.Error("Load failed [",i,"] (WebSocket Client) Error:",err)
		Setting.mu.Unlock()
		SendListenError(i)
		return
	}

	Setting.Listener.WSC[i] = ln
	Setting.mu.Unlock()

	for{
		conn,err := ln.Accept()

		if err != nil {
            if err, ok := err.(net.Error); ok && err.Temporary() {
                continue
            }
			break
		}

		Setting.mu.RLock()
        rule := Setting.Config.Rules[i]
		if Setting.Config.Users[rule.UserID].Used > Setting.Config.Users[rule.UserID].Quota { 			
			Setting.mu.RUnlock()
			conn.Close()
			continue
		}

		Setting.mu.RUnlock()

		if rule.Status != "Active" && rule.Status != "Created" {
			conn.Close()
			continue
		}

		ws,err :=websocket.Dial("ws://" + rule.Forward,"","http://" + rule.Forward)
		if err != nil {
			conn.Close()
			continue
		}

		go net_copyIO(conn,ws,i)
		go net_copyIO(ws,conn,i)
	}
}


func DeleteWSCRules(i string){
	if _,ok :=Setting.Listener.WSC[i];ok {
		err :=Setting.Listener.WSC[i].Close()
		for err!=nil {
		time.Sleep(time.Second)
		err = Setting.Listener.WSC[i].Close()
		}
	}
	Setting.mu.Lock()
	zlog.Info("Deleted [",i,"] (WebSocket Client)", Setting.Config.Rules[i].Listen, " => ", Setting.Config.Rules[i].Forward)
	delete(Setting.Config.Rules,i)
	delete(Setting.Listener.WSC,i)
	Setting.mu.Unlock()
}

