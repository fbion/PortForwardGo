package main

import (
	"errors"
	"net"
	"time"
	"io"
	"PortForwardGo/zlog"
	"wego/util/ratelimit"
)

type UDPDistribute struct {
	Established bool
	Conn        *(net.UDPConn)
	RAddr       net.Addr
	Cache       chan []byte
}

type Conn interface {
    Read(b []byte) (n int, err error)
    Write(b []byte) (n int, err error)
    Close() (error)
    RemoteAddr() (net.Addr)
}

func LoadUDPRules(i string){
	Setting.mu.Lock()

	addr, _ := net.ResolveUDPAddr("udp", ":"+Setting.Config.Rules[i].Listen)
	serv, err := net.ListenUDP("udp", addr)

	if err == nil {
		zlog.Info("Loaded [",i,"] (UDP) ", Setting.Config.Rules[i].Listen, " => ", Setting.Config.Rules[i].Forward)
	}else{
		zlog.Error("Load failed [",i,"] (UDP) Error: ",err)
		Setting.mu.Unlock()
		SendListenError(i)
		return
	}

	Setting.Listener.UDP[i] = serv
	Setting.mu.Unlock()

	
	table := make(map[string]*UDPDistribute)
	for {
		Setting.mu.RLock()
		_, ok := Setting.Config.Rules[i]
		if !ok {
			Setting.mu.RUnlock()
			return
		}
		Setting.mu.RUnlock()

		serv.SetDeadline(time.Now().Add(16 * time.Second))

		buf := make([]byte, 32 * 1024)
		n, addr, err := serv.ReadFrom(buf)
		if err != nil {
			continue
		}
		buf = buf[:n]

		if d, ok := table[addr.String()]; ok {
			if d.Established {
				d.Cache <- buf
				continue
			} else {
				delete(table, addr.String())
			}
		}
		conn := NewUDPDistribute(serv, addr)
		table[addr.String()] = conn
		conn.Cache <- buf

		Setting.mu.RLock()

	    if Setting.Config.Users[Setting.Config.Rules[i].UserID].Used > Setting.Config.Users[Setting.Config.Rules[i].UserID].Quota { // Check the quota
	    	Setting.mu.RUnlock()
		    conn.Close()
			continue
	    }
		if Setting.Config.Rules[i].Status != "Active" && Setting.Config.Rules[i].Status != "Created" {
		   	Setting.mu.RUnlock()
			conn.Close()
			continue
		}
		go udp_handleRequest(conn,i,Setting.Config.Rules[i])
		
		Setting.mu.RUnlock()
		
	}
}
		
func DeleteUDPRules(i string){
	if _,ok :=Setting.Listener.UDP[i];ok{
	err :=Setting.Listener.UDP[i].Close()
	for err!=nil {
	time.Sleep(time.Second)
	err = Setting.Listener.UDP[i].Close()
	}
    }
	Setting.mu.Lock()
	zlog.Info("Deleted [",i,"] (UDP)", Setting.Config.Rules[i].Listen, " => ", Setting.Config.Rules[i].Forward)
	delete(Setting.Config.Rules,i)
	delete(Setting.Listener.UDP,i)
	Setting.mu.Unlock()
}

func udp_handleRequest(conn Conn, index string, r Rule) {
    	proxy, err := ConnUDP(r.Forward)
		if err != nil {
			conn.Close()
			return
		}

		go udp_copyIO(conn,proxy,index)
		go udp_copyIO(proxy,conn,index)
}

func NewUDPDistribute(conn *(net.UDPConn), addr net.Addr) (*UDPDistribute) {
	return &UDPDistribute{
		Established: true,
		Conn:        conn,
		RAddr:       addr,
		Cache:       make(chan []byte, 16),
	}
}

func (this *UDPDistribute) Close() (error) {
	this.Established = false
	return nil
}

func (this *UDPDistribute) Read(b []byte) (n int, err error) {
	if !this.Established {
		return 0, errors.New("udp distrubute has closed")
	}

	select {
	case <-time.After(16 * time.Second):
		return 0, errors.New("udp distrubute read timeout")
	case data := <-this.Cache:
		n := len(data)
		copy(b, data)
		return n, nil
	}
}

func (this *UDPDistribute) Write(b []byte) (n int, err error) {
	if !this.Established {
		return 0, errors.New("udp distrubute has closed")
	}
	return this.Conn.WriteTo(b, this.RAddr)
}

func (this *UDPDistribute) RemoteAddr() (net.Addr) {
	return this.RAddr
}

func ConnUDP(address string) (Conn, error) {
	conn, err := net.DialTimeout("udp", address, 10 * time.Second)
	if err != nil {
		return nil, err
	}

	_, err = conn.Write([]byte("\x00"))
	if err != nil {
		return nil, err
	}

	conn.SetDeadline(time.Now().Add(60 * time.Second))
	return conn, nil
}

func udp_copyIO(src,dest Conn,index string) {
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
