package main

import (
    "bufio"
    "flag"
    "fmt"
    "os"
    "os/exec"
    "strconv"
    "strings"
    "time"
)

type Proxies map[int]Proxy

func ParseProxyFile(path string, encrypted bool) (self Proxies, err error) {
    file, err := os.Open(path)
    if err != nil { return }
    defer file.Close()
    
    self = map[int]Proxy {}
    
    scanner := bufio.NewScanner(file)
    for line_num := 0; scanner.Scan(); line_num += 1 {
        line := strings.TrimSpace(scanner.Text())
        
        // Ignore comment lines
        if strings.HasPrefix(line, "#") || line == "" {
            continue
        }
        
        // Allow comments on partial lines
        content := strings.TrimSpace(strings.Split(line, "#")[0])
        
        self[line_num], err = ParseProxy(content, encrypted)
        if err != nil {
            err = fmt.Errorf("%s: line %d: %s", path, line_num, err)
            return
        }
    }
    return
}

type Proxy struct {
    LocalPort  int64
    Remote     string
    RemotePort int64
    Encrypted  bool
}

func ParseProxy(s string, encrypted bool) (self Proxy, err error) {
    parts := strings.Split(s, ":")
    
    self.LocalPort, err = strconv.ParseInt(parts[0], 0, 64)
    if err != nil { return }
    self.Remote = parts[1]
    self.RemotePort, err = strconv.ParseInt(parts[2], 0, 64)
    self.Encrypted = encrypted
    return
}

func (self *Proxy) Cmd() *exec.Cmd {
    if self.Encrypted {
        return exec.Command("ssh",
            "-o", "BatchMode=yes",
            "-L", self.String(),
            self.Remote,
        )
    } else {
        tcp_src := fmt.Sprintf("tcp-listen:%d,reuseaddr,fork", self.LocalPort)
        tcp_sink := fmt.Sprintf("tcp:%s:%d", self.Remote, self.RemotePort)
        return exec.Command("socat", tcp_src, tcp_sink)
    }
}

func (self *Proxy) Run(id int, deaths chan int) {
    fmt.Printf("Starting Proxy: %s\n", self)
    cmd := self.Cmd()
    out, err := cmd.CombinedOutput()
    if err != nil {
        fmt.Printf("Warn: proxy failed: %s\n", err)
        if len(out) > 0 {
            fmt.Printf("Stdout + Stderr was:\n%s", out)
        }
    }
    
    deaths <- id
}

func (self Proxy) String() string {
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
    
    ssh := flag.Bool(
        "e",
        false,
        "Set up an encrypted tunnel using ssh instead of plain tcp (the default)\n" +
        "ssh needs to be configured to connect to all the hosts in the proxy list",
    )
    file := flag.String(
        "p",
        "proxies.list",
        "file with a list of proxies to ensure\n" +
        "Format:\n" +
        "  Comment lines begin with '#'\n" +
        "  Other lines are formatted as <local_port>:<remote>:<remote_port>\n",
    )
    flag.Parse()
    
    proxies, err := ParseProxyFile(*file, *ssh)
    if err != nil { return }
    
    dead := make(chan int, len(proxies))
    
    // Push all the IDs to the channel to indicate that they are dead and need to be started
    for id, _ := range proxies {
        dead <- id
    }
    
    recent_restarts := map[int]int {}
    
    for died := range dead {
        restart_count, restarted := recent_restarts[died]
        if restarted {
            recent_restarts[died] += 1
        } else {
            recent_restarts[died] = 1
        }
        
        proxy := proxies[died]
        if restart_count <= 3 {
            go proxy.Run(died, dead)
        } else {
            fmt.Printf("Too many restarts for %s, sleeping restart loop\n", proxy)
            delete(recent_restarts, died)
            // Wait a while before we try restarting again
            go func() {
                time.Sleep(2 * time.Minute)
                proxy.Run(died, dead)
            }()
        }
    }
}

