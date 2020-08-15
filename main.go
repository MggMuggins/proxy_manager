package main

import (
    "flag"
    "fmt"
    "os"
    "os/exec"
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
    
    //ssh := args.Bool("e", false, "Set up an encrypted tunnel using ssh instead of plain tcp")
    cfg_file := flag.String("p", "proxies.toml", "TOML file with list of proxies to ensure")
    flag.Parse()
    
    cfg, err := NewConfig(*cfg_file)
    if err != nil { return }
    
    proxies := map [int]Proxy {}
    dead := make(chan int, len(cfg.Proxy))
    
    // Set up the main thread's map of proxies with their IDs, and push all the
    //  IDs to the channel to indicate that they are dead and need to be started
    for i, proxy := range cfg.Proxy {
        proxies[i] = proxy
        dead <- i
    }
    
    // Wait on the channel to get any daemons that need to be started
    for {
        died := <-dead
        proxy := proxies[died]
        go proxy.Run(died, dead)
    }
}

