/*
   @author Joe Williams
   
   Reverse Proxy / Load Balancer

   Add ""

*/

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"net"
	"os/exec"
	"strings"
	"sync"
	"math/rand"
	"time"
)

// Multiplexer Proxy
type MuxProxy struct {
	// callbacks
	TargetSelector func(*http.Request) (*url.URL)
	Started func()
	Sending func(http.ResponseWriter, *http.Request)
	ModifyResponse func(*http.Response) (error)
	// variables
	urls    []*url.URL
	proxy *httputil.ReverseProxy
	mode	string
	index   int
	Len     int
}

// Settings for the Multiplexer Proxy
type Settings struct {
	Host     string   `json:"host"`
	Protocol string   `json:"protocol"`
	Format   string   `json:"format"`
	Ports    []string `json:"ports"`
}

// Create a new Multiplexer Proxy, and initializes the data with an array of URLs
func NewMuxProxy(rawurls []string) *MuxProxy {
	length := len(rawurls)
	urls := make([]*url.URL, length)
	for i, rawurl := range rawurls {
		u, e := url.Parse(rawurl)
		if e != nil {
			log.Fatal("URL Parse Error", e)
		}
		urls[i] = u
	}
	mp := &MuxProxy{urls: urls, mode: "default", Len: length}
	return NewMultipleHostReverseProxy(mp)
}

func NewMultipleHostReverseProxy(mp *MuxProxy) (*MuxProxy) {
	director := func(r *http.Request) {
		var target *url.URL
		switch(mp.mode) {
			case "custom":
				if mp.TargetSelector != nil {
					target = mp.TargetSelector(r)
				} else {
					panic("Custom target selector not defined")
				}
			case "random":
				target = mp.urls[rand.Int() % len(mp.urls)]
			case "default":
				log.Println("Sending " + string())
				target = mp.urls[mp.index % len(mp.urls)]
				mp.index++
		}
		r.URL.Scheme = target.Scheme
		r.URL.Host = target.Host
		r.URL.Path = target.Path
	}

	mp.proxy = &httputil.ReverseProxy{
		Director: director,
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return http.ProxyFromEnvironment(req)
			},
			Dial: func(network, addr string) (net.Conn, error) {
				conn, err := (&net.Dialer{
						Timeout:   5 * time.Second,
						KeepAlive: 10 * time.Second,
				}).Dial(network, addr)
				if err != nil {
						println("Error during DIAL:", err.Error())
				}
				return conn, err
			},
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	if mp.Started != nil {
		mp.Started()
	}
	return mp
}

// Proxy to server
func (mp *MuxProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if mp.Sending != nil {
		mp.Sending(w,r)
	}
	mp.proxy.ServeHTTP(w, r)
}

// Switches between proxies
func (mp *MuxProxy) Switcher() int {
	mp.index += 1
	if mp.index == mp.Len {
		mp.index = 0
	}
	return mp.index
}

// Runs a command and prints out it's output
func RunCommand(cmd string, wg *sync.WaitGroup) {
	defer wg.Done()
	args := strings.Fields(cmd)
	id := args[2:3]
	app := exec.Command(args[0], args[1:]...)
	stdout, err := app.StdoutPipe()
	stderr, err := app.StderrPipe()
	if err != nil {
		log.Fatal(id, err)
	}
	if err := app.Start(); err != nil {
		log.Fatal(id, err)
	}
	// Prints out the output and errors for the app
	go ReadWrite(id, stdout)
	go ReadWrite(id, stderr)
	// Waits for the app to close
	app.Wait()
	fmt.Println(id, "Restarting Server")
	//go RunCommand(cmd, wg)
}

// Reads from the io.Reader and outputs the data with the id as the Header
func ReadWrite(id []string, out io.Reader) {
	rdr := bufio.NewReader(out)
	line := ""
	for {
		// Reads a line from the output
		buf, part, err := rdr.ReadLine()
		if err != nil {
			// Error Checking
			if err == io.EOF {
				continue
			}
			break
		}
		// Adds the line to temp variable
		line += fmt.Sprintf("%s", buf)
		// If the line isn't partial, print the temp var
		// else keep adding to the temp var
		if !part {
			fmt.Printf("%s> %s\n", id, line)
			line = ""
		}
	}
}

func MuxProxyParseJSON() (*MuxProxy, *sync.WaitGroup) {
	// Parse files
	filedata, err := ioutil.ReadFile("settings.json")
	if err != nil {
		panic(err)
	}
	// JSON to Struct
	var options Settings
	if err := json.Unmarshal(filedata, &options); err != nil {
		panic(err)
	}
	// Dynamic programming stuff
	var wg sync.WaitGroup
	num_ports := len(options.Ports)
	cmds := make([]string, num_ports)
	urls := make([]string, num_ports)
	for i := 0; i < num_ports; i++ {
		urls[i] = fmt.Sprintf("%s://%s:%s",
			options.Protocol,
			options.Host,
			options.Ports[i])
		cmds[i] = fmt.Sprintf(options.Format,
			options.Ports[i])
		wg.Add(1)
		go RunCommand(cmds[i], &wg)
	}
	return NewMuxProxy(urls), &wg
}

func InitMuxProxy() {
	mp, wg := MuxProxyParseJSON()
	http.Handle("/", mp)
	go http.ListenAndServe(":8080", nil)
	wg.Wait()
}

func main() {
	// Server stuff
	// fs := http.FileServer(http.Dir("public"))
	// http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
	// 	http.ServeFile(w, r, "favicon.ico")
	// })
	// http.Handle("/assets/", http.StripPrefix("/assets", fs))
	mp := NewMuxProxy([]string{"https://github.com","https://www.yahoo.com"})
	http.Handle("/", mp)
	log.Printf("Load balancing on 8080 to %d different processes", mp.Len)
	http.ListenAndServe(":8080", nil)
}
