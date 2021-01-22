package main

import (
	"PortForwardGo/zlog"
	"net/http"
	"net"
	"time"
	"golang.org/x/net/websocket"
)

func LoadWSRules(i string){
	Setting.mu.Lock()
	tcpaddress, _ := net.ResolveTCPAddr("tcp", ":"+Setting.Config.Rules[i].Listen)
	ln, err := net.ListenTCP("tcp", tcpaddress)
	if err == nil {
		zlog.Info("Loaded [",i,"] (WebSocket)", Setting.Config.Rules[i].Listen, " => ", Setting.Config.Rules[i].Forward)
	}else{
		zlog.Error("Load failed [",i,"] (Websocket) Error: ",err)
		SendListenError(i)
		Setting.mu.Unlock()
		return
	}
	Setting.Listener.WS[i] = ln
	Setting.mu.Unlock()
	http.Serve(ln,websocket.Handler(func(ws *websocket.Conn){WS_Handle(i,ws)}))
}

func DeleteWSRules(i string){
	if _,ok :=Setting.Listener.WS[i];ok {
		err :=Setting.Listener.WS[i].Close()
		for err!=nil {
		time.Sleep(time.Second)
		err = Setting.Listener.WS[i].Close()
		}
	}
	Setting.mu.Lock()
	zlog.Info("Deleted [",i,"] (WebSocket)", Setting.Config.Rules[i].Listen, " => ", Setting.Config.Rules[i].Forward)
	delete(Setting.Config.Rules,i)
	delete(Setting.Listener.WS,i)
	Setting.mu.Unlock()
}


func WS_Handle(i string , ws *websocket.Conn){
    Setting.mu.RLock()
	
	if Setting.Config.Users[Setting.Config.Rules[i].UserID].Used > Setting.Config.Users[Setting.Config.Rules[i].UserID].Quota { 			
		Setting.mu.RUnlock()
		ws.Close()
		return
	}

	if Setting.Config.Rules[i].Status != "Active" && Setting.Config.Rules[i].Status != "Created" {
		Setting.mu.RUnlock()
		ws.Close()
		return
	}

	dest := Setting.Config.Rules[i].Forward
    Setting.mu.RUnlock()

   conn,err := net.Dial("tcp",dest)
   if err != nil {
	   ws.Close()
	   return
   }

   go net_copyIO(ws,conn,i)
   go net_copyIO(conn,ws,i)
}