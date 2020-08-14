package main

import (
    "fmt"
    "os"
    "os/exec"
    //"time"
    "github.com/pelletier/go-toml"
)

type Config struct {
    Proxy []Proxy
}



func NewConfig(path string) (cfg Config, err error) {
    cfg = Config {}
    
    cfg_file, err := os.Open(path)
    defer cfg_file.Close()
    if err != nil { return }
    
    decoder := toml.NewDecoder(cfg_file)
    
    err = decoder.Decode(&cfg)
    if err != nil {
        err = fmt.Errorf("failed to parse %s: %s", path, err)
    }
    return
}

type Proxy struct {
    LocalPort  int
    Remote     string
    RemotePort int
}

func (self *Proxy) Run(id int, deaths chan int) {
    //time.Sleep(10 * time.Second)
    tcp_src := fmt.Sprintf("tcp-listen:%d,reuseaddr,fork", self.LocalPort)
    tcp_sink := fmt.Sprintf("tcp:%s:%d", self.Remote, self.RemotePort)
    
    fmt.Printf("Starting Proxy: %s\n", self.Fmt())
    cmd := exec.Command("socat", tcp_src, tcp_sink)
    err := cmd.Run()
    if err != nil {
        fmt.Printf("Warn: proxy failed: %s\n", err)
    }
    
    deaths <- id
}

func (self *Proxy) Fmt() (string) {
    return fmt.Sprintf("%d:%s:%d", self.LocalPort, self.Remote, self.RemotePort)
}

func main() {
    var err error
    defer func() {
        if err != nil {
            fmt.Printf("error: %s\n", err)
            os.Exit(1)
        }
    }()
    
    cfg, err := NewConfig("proxies.toml")
    if err != nil { return }
    
    proxies := map[int]Proxy{}
    deaths := make(chan int, len(cfg.Proxy))
    
    for i, proxy := range cfg.Proxy {
        proxies[i] = proxy
        go proxy.Run(i, deaths)
    }
    
    for {
        died := <-deaths
        proxy := proxies[died]
        go proxy.Run(died, deaths)
    }
}

