package main

import (
    "bufio"
    "fmt"
    "os"
    "os/exec"
    "strconv"
    "strings"
    "time"
    
    "github.com/akamensky/argparse"
    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"
)

const (
    ARG_HELP = "Start and maintain tcp or ssh port forwards"
    ARG_HELP_SSH = "Use ssh instead of plain tcp (the default) for port-forwarding"
    ARG_HELP_PROXIES = "File with a list of proxies to ensure"
    BACKOFF_SLEEP = 2 * time.Minute
    RESTART_MAX = 3
)

type Proxies map[int]Proxy

func ParseProxyFile(path string, encrypted bool) (self Proxies, err error) {
    file, err := os.Open(path)
    if err != nil { return }
    defer file.Close()
    
    self = map[int]Proxy {}
    
    scanner := bufio.NewScanner(file)
    for line_num := 1; scanner.Scan(); line_num += 1 {
        line := strings.TrimSpace(scanner.Text())
        
        if strings.HasPrefix(line, "#") || line == "" {
            continue
        }
        
        // Allow comments on partial lines
        content := strings.TrimSpace(strings.Split(line, "#")[0])
        
        // Use the line num for the id; no sense iterating again
        self[line_num], err = ParseProxy(content, line_num, encrypted)
        if err != nil {
            err = fmt.Errorf("%s: line %d: %s", path, line_num, err)
            return
        }
    }
    return
}

type Proxy struct {
    // proxy file fields
    LocalPort  int64
    Remote     string
    RemotePort int64
    
    // meta runtime fields
    Encrypted  bool
    Id         int
}

func ParseProxy(s string, id int, encrypted bool) (self Proxy, err error) {
    self.Encrypted = encrypted
    self.Id = id
    
    parts := strings.Split(s, ":")
    
    self.LocalPort, err = strconv.ParseInt(parts[0], 0, 64)
    if err != nil { return }
    
    self.Remote = parts[1]
    if self.Remote == "" {
        err = fmt.Errorf("Remote was empty")
        return
    }
    
    self.RemotePort, err = strconv.ParseInt(parts[2], 0, 64)
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

func (self *Proxy) Run(deaths chan int) {
    log.Info().Str("Proxy", self.String()).
        Msg("Starting")
    
    cmd := self.Cmd()
    log.Debug().Strs("Args", cmd.Args).Msg("Spawning Process")
    out, err := cmd.CombinedOutput()
    if err != nil {
        log.Warn().Err(err).
            Msg("Proxy failed")
        
        if len(out) > 0 {
            log.Info().Str("StdoutAndStderr", string(out)).Msg("")
        }
    }
    
    deaths <- self.Id
}

func (self Proxy) String() string {
    return fmt.Sprintf("%d:%s:%d", self.LocalPort, self.Remote, self.RemotePort)
}

func main() {
    log.Logger = log.Output(zerolog.ConsoleWriter { Out: os.Stderr })
    
    var err error
    defer func() {
        if err != nil {
            log.Fatal().Err(err).Msg("Failed to start daemon")
        }
    }()
    
    parser := argparse.NewParser("proxy_manager", ARG_HELP)
    ssh := parser.Flag("s", "secure", &argparse.Options { Help: ARG_HELP_SSH })
    dbg := parser.FlagCounter("v", "verbose", &argparse.Options { Help: "Enable verbose logging" })
    file := parser.String("p", "proxies", &argparse.Options { Default: "proxies.list", Help: ARG_HELP_PROXIES })
    err = parser.Parse(os.Args)
    if err != nil {
        err = fmt.Errorf("%s, use -h for help", err)
        return
    }
    
    zerolog.SetGlobalLevel(zerolog.InfoLevel - zerolog.Level(*dbg))
    
    proxies, err := ParseProxyFile(*file, *ssh)
    if err != nil { return }
    
    // Proxy.Run() posts its ID to this channel when its subprocess ends
    dead := make(chan int, len(proxies))
    
    // Push all the IDs to the channel to indicate that they are dead and need to be started
    for id, _ := range proxies {
        dead <- id
    }
    
    recent_restarts := map[int]int {}
    
    for died := range dead {
        recent_restarts[died] += 1
        proxy := proxies[died]
        
        if recent_restarts[died] <= RESTART_MAX {
            go proxy.Run(dead)
        } else {
            log.Warn().
                Str("Proxy", proxy.String()).
                Dur("BackoffFor", BACKOFF_SLEEP).
                Int("MaxRestarts", RESTART_MAX).
                Msg("Too many restarts")
            delete(recent_restarts, died)
            
            go func() {
                // Wait a while before we try restarting again
                time.Sleep(BACKOFF_SLEEP)
                proxy.Run(dead)
            }()
        }
    }
}

