package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"bytes"
	"net"
	"os"
	"fmt"
	"os/signal"
	"sync"
	"syscall"
	"crypto/md5"
	"encoding/hex"
	"time"
	"PortForwardGo/zlog"
	"flag"
)

var Setting CSafeRule

const Version = "1.1.8"

var ConfigFile string
var LogFile string

type CSafeRule struct {
	Listener Listener
    Config Config
	mu    sync.RWMutex
}

type Listener struct{
	TCP map[string]*net.TCPListener
	UDP map[string]*net.UDPConn
	WS map[string]*net.TCPListener
	WSC map[string]*net.TCPListener
}

type Config struct{
	UpdateInfoCycle int
	EnableAPI bool
	APIPort string
	Listen map[string]Listen
	Rules map[string]Rule
	Users map[string]User
}

type Listen struct{
	Enable bool
	Port string
}

type User struct{
	Speed int64
	Quota int64
	Used int64
}

type Rule struct{
	Status string
	UserID string
	Protocol string
	Listen string
	Forward string
}

type APIConfig struct {
	APIAddr string
	APIToken string
	NodeID int
}

var apic APIConfig

func main() {
		flag.StringVar(&ConfigFile, "config", "config.json", "The config file location.")
		flag.StringVar(&LogFile,"log","run.log","The log file location.")
		help := flag.Bool("h", false, "Show help")
		flag.Parse()

		if *help {
			flag.PrintDefaults()
			os.Exit(0)
		}
	

		zlog.Info("Node Version: ",Version)
		logfile_writer,err := os.OpenFile(LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
        if err == nil{
		zlog.SetOutput(logfile_writer)
		zlog.Info("Log file location: ",LogFile)
		}

		LoadMap()

    apif, err := ioutil.ReadFile(ConfigFile)
		if err != nil {
			zlog.Fatal("Cannot read the config file. (io Error) " + err.Error())
		}
   err = json.Unmarshal(apif,&apic)
   if err != nil {
	zlog.Fatal("Cannot read the config file. (Parse Error) " + err.Error())
   }

    zlog.Info("API URL: ",apic.APIAddr)   
	GetRules()

	for index, _ := range Setting.Config.Rules {
		go func(index string){
			LoadNewRules(index)
		}(index)
	}

	go func(){
		if Setting.Config.EnableAPI == true {
		zlog.Info("[HTTP API] Listening " , Setting.Config.APIPort," Path: /",md5_encode(apic.APIToken)," Method:POST")
	    route := http.NewServeMux()
	    route.HandleFunc("/" + md5_encode(apic.APIToken), NewAPIConnect)
	    err := http.ListenAndServe(":" + Setting.Config.APIPort,route)
        if err != nil {
        zlog.Error("[HTTP API] ", err)
        }
	  }
    }()

	go func(apic APIConfig) {
		for {
	      	saveInterval := time.Duration(Setting.Config.UpdateInfoCycle) * time.Second
			time.Sleep(saveInterval)
			updateConfig()
		}
	}(apic)

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		done <- true
	}()
	<-done
	saveConfig()
	zlog.PrintText("Exiting\n")
}

func NewAPIConnect(w http.ResponseWriter, r *http.Request){
	var NewConfig Config
	if r.Method != "POST" {
		io.WriteString(w,"Unsupport Method.")
		return
	}
	postdata,_ := ioutil.ReadAll(r.Body)
	err := json.Unmarshal(postdata,&NewConfig)
	if(err != nil){
	   io.WriteString(w,fmt.Sprintln(err))
	   return
	}
	
	Setting.mu.Lock()
	if Setting.Config.Rules == nil{
	Setting.Config.Rules = make(map[string]Rule)
	}

	if Setting.Config.Users == nil{
		Setting.Config.Users = make(map[string]User)
	}

	for index,_ := range NewConfig.Users {
		Setting.Config.Users[index] = NewConfig.Users[index]
	}
    
	for index, _ := range NewConfig.Rules {
	    if NewConfig.Rules[index].Status == "Deleted" {
			go func(index string){
			DeleteRules(index)
			}(index)
			break
    	}else if NewConfig.Rules[index].Status == "Created" {
			Setting.Config.Rules[index] = NewConfig.Rules[index]
			go func(index string){
			LoadNewRules(index)
			}(index)
			break
		}else{
			Setting.Config.Rules[index] = NewConfig.Rules[index]
			break
		}
	}
	Setting.mu.Unlock()
	io.WriteString(w,"Success")
	return
}

func LoadMap(){
	Setting.mu.Lock()
	Setting.Listener.TCP = make(map[string]*net.TCPListener)
	Setting.Listener.UDP = make(map[string]*net.UDPConn)
	Setting.Listener.WS = make(map[string]*net.TCPListener)
	Setting.Listener.WSC = make(map[string]*net.TCPListener)
	Setting.mu.Unlock()
}

func LoadListen(){
	for name , value := range Setting.Config.Listen{
		if value.Enable{
		switch name {
		 case "Http":
			go HttpInit()
		 case "Https":
			go HttpsInit()
		 }
	  }
	}
}

func DeleteRules(i string){
if _,ok := Setting.Config.Rules[i];ok{
	Protocol := Setting.Config.Rules[i].Protocol
	if Protocol == "tcp" {
		go DeleteTCPRules(i)
    }else if Protocol == "udp" {
		go DeleteUDPRules(i)
	}else if Protocol == "http" {
		go DeleteHttpRules(i)
	}else if Protocol == "https" {
		go DeleteHttpsRules(i)
	}else if Protocol == "ws" {
		go DeleteWSRules(i)
	}else if Protocol == "wsc" {
		go DeleteWSCRules(i)
	}
}
}

func LoadNewRules(i string){
	Protocol := Setting.Config.Rules[i].Protocol

	if Protocol == "tcp" {
    	LoadTCPRules(i)
	}else if Protocol == "udp" {
    	LoadUDPRules(i)
	}else if Protocol == "http" {
	    LoadHttpRules(i)
	}else if Protocol == "https" {
	    LoadHttpsRules(i)
	}else if Protocol == "https" {
		LoadHttpsRules(i)
	}else if Protocol == "ws" {
		LoadWSRules(i)
	}else if Protocol == "wsc" {
		LoadWSCRules(i)
	}
}

func updateConfig() {
	var NewConfig Config
	Setting.mu.Lock()

	jsonData,_ := json.Marshal(map[string]interface{}{
		"Action" : "UpdateInfo",
		"NodeID" : apic.NodeID,
		"Token" : md5_encode(apic.APIToken),
		"Info" : &Setting.Config,
		"Version" : Version,
	})
	status,confF,err := sendRequest(apic.APIAddr,bytes.NewReader(jsonData),nil,"POST")
	if status == 503 {
		zlog.Error("Scheduled task update error,The remote server returned an error message: ", string(confF))
		Setting.mu.Unlock()
		return
	}
	if err != nil {
		zlog.Error("Scheduled task update: ", err)
		Setting.mu.Unlock()
		return
	} 
	
	err = json.Unmarshal(confF, &NewConfig)
	if err != nil {
		zlog.Error("Cannot read the port forward config file. (Parse Error) " + err.Error())
		Setting.mu.Unlock()
		return
	}
	Setting.Config = NewConfig
	for index, _ := range Setting.Config.Rules {
		if Setting.Config.Rules[index].Status == "Deleted" {
			go func(index string){
				DeleteRules(index)
			}(index)
	    }else if Setting.Config.Rules[index].Status == "Created" {
			go func(index string){
			LoadNewRules(index)
			}(index)
		}
	}
	Setting.mu.Unlock()
	zlog.Success("Scheduled task update Completed")
}

func saveConfig() {
	Setting.mu.Lock()

	jsonData,_ := json.Marshal(map[string]interface{}{
		"Action" : "SaveConfig",
		"NodeID" : apic.NodeID,
		"Token" : md5_encode(apic.APIToken),
		"Info" : &Setting.Config,
		"Version" : Version,
	})
	status,confF,err := sendRequest(apic.APIAddr,bytes.NewReader(jsonData),nil,"POST")
	if status == 503 {
		Setting.mu.Unlock()
		zlog.Error("Save config error,The remote server returned an error message , message: ", string(confF))
		return
	}
	if err != nil {
		zlog.Error("Save config error: ", err)
		Setting.mu.Unlock()
		return
	}
	
	Setting.mu.Unlock()
	zlog.Success("Save config Completed")
}


func SendListenError(i string){
	jsonData,_ := json.Marshal(map[string]interface{}{
		"Action" : "Error",
		"NodeID" : apic.NodeID,
		"Token" : md5_encode(apic.APIToken),
		"Version" : Version,
		"RuleID" : i,
	}) 
	sendRequest(apic.APIAddr,bytes.NewReader(jsonData),nil,"POST")
}

func GetRules(){
	var NewConfig Config
    Setting.mu.Lock()
	jsonData,_ := json.Marshal(map[string]interface{}{
		"Action" : "GetConfig",
		"NodeID" : apic.NodeID,
		"Token" : md5_encode(apic.APIToken),
		"Version" : Version,
	})
	status,confF,err := sendRequest(apic.APIAddr,bytes.NewReader(jsonData),nil,"POST")
	if status == 503 {
		Setting.mu.Unlock()
		zlog.Error("The remote server returned an error message: ", string(confF))
		return
	}

	if err != nil {
		Setting.mu.Unlock()
		zlog.Fatal("Cannot read the online config file. (NetWork Error) " + err.Error())
		return
	}

	err = json.Unmarshal(confF, &NewConfig)
	if err != nil {
		Setting.mu.Unlock()
		zlog.Fatal("Cannot read the port forward config file. (Parse Error) " + err.Error())
		return
	}
	Setting.Config = NewConfig
	zlog.Info("Update Cycle: ",Setting.Config.UpdateInfoCycle," seconds")
	Setting.mu.Unlock()
	LoadListen()
}

func sendRequest(url string, body io.Reader, addHeaders map[string]string, method string) (statuscode int,resp []byte,err error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return
	}

	if len(addHeaders) > 0 {
		for k, v := range addHeaders {
			req.Header.Add(k, v)
		}
	}

	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return
	}
	defer response.Body.Close()

	statuscode = response.StatusCode
	resp, err = ioutil.ReadAll(response.Body)
	return
}

func md5_encode(s string)string{
	h :=md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}