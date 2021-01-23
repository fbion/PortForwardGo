package main

import (
	"PortForwardGo/zlog"
	"net"
	"io"
	"wego/util/ratelimit"
	"time"
)

func LoadTCPRules(i string) {
	Setting.mu.Lock()
	tcpaddress, _ := net.ResolveTCPAddr("tcp", ":"+Setting.Config.Rules[i].Listen)
	ln, err := net.ListenTCP("tcp", tcpaddress)
	if err == nil {
		zlog.Info("Loaded [",i,"] (TCP)", Setting.Config.Rules[i].Listen, " => ", Setting.Config.Rules[i].Forward)
	}else{
		zlog.Error("Load failed [",i,"] (TCP) Error: ",err)
		SendListenError(i)
		Setting.mu.Unlock()
		return
	}
	Setting.Listener.TCP[i] = ln
	Setting.mu.Unlock()
	for {
		conn, err := ln.Accept()
		
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

		go tcp_handleRequest(conn, i, Setting.Config.Rules[i])
		Setting.mu.RUnlock()
	}
}

func DeleteTCPRules(i string){
	if _,ok :=Setting.Listener.TCP[i];ok {
		err :=Setting.Listener.TCP[i].Close()
		for err!=nil {
		time.Sleep(time.Second)
		err = Setting.Listener.TCP[i].Close()
		}
	}
	Setting.mu.Lock()
	zlog.Info("Deleted [",i,"] (TCP)", Setting.Config.Rules[i].Listen, " => ", Setting.Config.Rules[i].Forward)
	delete(Setting.Config.Rules,i)
	delete(Setting.Listener.TCP,i)
	Setting.mu.Unlock()
}

func tcp_handleRequest(conn net.Conn, index string, r Rule) {
	proxy, err := net.Dial(r.Protocol, r.Forward)
	if err != nil {
		_ = conn.Close()
		return
	}

	go net_copyIO(conn, proxy, index)
	go net_copyIO(proxy, conn, index)
}

func net_copyIO(src, dest net.Conn, index string) {
	defer src.Close()
	defer dest.Close()

	var r int64
	var userid string

	Setting.mu.RLock()
	userid = Setting.Config.Rules[index].UserID
	if Setting.Config.Users[userid].Speed != 0{
	bucket := ratelimit.New(Setting.Config.Users[userid].Speed * 128 * 1024)
	Setting.mu.RUnlock()
	r, _ = io.Copy(ratelimit.Writer(dest,bucket),src)
	}else{
	Setting.mu.RUnlock()
	r, _ = io.Copy(dest, src)
    }
    Setting.mu.Lock()
	NowUser :=Setting.Config.Users[userid]
	NowUser.Used += r
	Setting.Config.Users[userid] = NowUser
	Setting.mu.Unlock()
}
