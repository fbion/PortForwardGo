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

		Setting.mu.RLock()

		if rule, ok := Setting.Config.Rules[i];!ok || rule.Status == "Deleted" {
			Setting.mu.RUnlock()
			break
		}

		if err != nil {
			Setting.mu.RUnlock()
			continue
		}

		if Setting.Config.Users[Setting.Config.Rules[i].UserID].Used > Setting.Config.Users[Setting.Config.Rules[i].UserID].Quota { 			
			Setting.mu.RUnlock()
			conn.Close()
			continue
		}

		if Setting.Config.Rules[i].Status != "Active" && Setting.Config.Rules[i].Status != "Created" {
			Setting.mu.RUnlock()
			conn.Close()
			continue
		}

		Setting.mu.RUnlock()

        dest := Setting.Config.Rules[i].Forward
		ws,err :=websocket.Dial("ws://"+dest,"","http://"+dest)
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

