package main

import (
	"bufio"
	"PortForwardGo/zlog"
	"net"
	"container/list"
	"strings"
)

var http_index map[string]string

const Page503 = ""

func HttpInit(){
	http_index = make(map[string]string)
	zlog.Info("[HTTP] Listening ",Setting.Config.Listen["Http"].Port)
	l, err := net.Listen("tcp",":"+Setting.Config.Listen["Http"].Port)
	if err != nil {
		zlog.Error("[HTTP] Listen failed , Error: ",err)
		return
	}
	for {
		c, err := l.Accept()
		if err != nil {
			continue
		}
		go http_handle(c)
	}
}

func LoadHttpRules(i string){
	Setting.mu.RLock()
	zlog.Info("Loaded [",i,"] (HTTPS)", Setting.Config.Rules[i].Listen, " => ", Setting.Config.Rules[i].Forward)
	http_index[strings.ToLower(Setting.Config.Rules[i].Listen)] = i
	Setting.mu.RUnlock()
}

func DeleteHttpRules(i string){
	Setting.mu.Lock()
	zlog.Info("Deleted [",i,"] (HTTP)", Setting.Config.Rules[i].Listen, " => ", Setting.Config.Rules[i].Forward)
	delete(http_index,strings.ToLower(Setting.Config.Rules[i].Listen))
	delete(Setting.Config.Rules,i)
	Setting.mu.Unlock()
}

func http_handle(conn net.Conn) {
	headers := bufio.NewReader(conn)
	hostname := ""
	readLines := list.New()
	for {
		bytes, _, error := headers.ReadLine()
		if error != nil {
			conn.Close()
			return
		}
		line := string(bytes)
		readLines.PushBack(line)
		
		if line == "" {
						break
		}

		if strings.HasPrefix(line, "X-Forward-For: ") == false {
			readLines.PushBack("X-Forward-For: "+ ParseAddrToIP(conn.RemoteAddr().String()))
		}

		if strings.HasPrefix(line, "Host: ") {
			hostname = strings.ToLower((strings.TrimPrefix(line, "Host: ")))
		}
	}
	
	if hostname == "" {
		conn.Close()
		return
	}

	i,ok := http_index[hostname]
	if !ok {
		conn.Write([]byte(Page503))
		conn.Close()
		return
	}

	Setting.mu.RLock()       	
	_, ok = Setting.Config.Rules[i]
	if !ok {
		conn.Write([]byte(Page503))
		conn.Close()
		Setting.mu.RUnlock()
		delete(http_index,i)
		return
	}
	if Setting.Config.Users[Setting.Config.Rules[i].UserID].Used > Setting.Config.Users[Setting.Config.Rules[i].UserID].Quota { 		Setting.mu.RUnlock()
		conn.Close()
		Setting.mu.RUnlock()	
		return
	}
	if Setting.Config.Rules[i].Status != "Active" && Setting.Config.Rules[i].Status != "Created" {
		Setting.mu.RUnlock()
		conn.Close()
		return
	}
	dest :=Setting.Config.Rules[i].Forward

    Setting.mu.RUnlock()

	backend, error := net.Dial("tcp", dest)
	if error != nil {
		conn.Close()
		return
	}

	for element := readLines.Front(); element != nil; element = element.Next() {
		line := element.Value.(string)
		backend.Write([]byte(line))
		backend.Write([]byte("\n"))
	}

	
	go net_copyIO(conn, backend,i)
	go net_copyIO(backend, conn,i)
}

func ParseAddrToIP(addr string) string {
	var str string
	arr :=strings.Split(addr,":")
        for i :=0;i< (len(arr) - 1);i++{
			if i!=0{
			str = str + ":" + arr[i]
			}else{
			str = str + arr[i]
			}
        }
    return str
}

func ParseHostToName(host string) string {
	if strings.Index(host,":") == -1{
        return host
	}else{
    	return strings.Split(host,":")[0]
	}
}