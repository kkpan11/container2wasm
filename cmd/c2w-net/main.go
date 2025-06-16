package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	gvntypes "github.com/containers/gvisor-tap-vsock/pkg/types"
	gvnvirtualnetwork "github.com/containers/gvisor-tap-vsock/pkg/virtualnetwork"
	"golang.org/x/net/websocket"
)

const (
	gatewayIP = "192.168.127.1"
	vmIP      = "192.168.127.3"
	vmMAC     = "02:00:00:00:00:01"
)

func main() {
	var portFlags sliceFlags
	flag.Var(&portFlags, "p", "map port between host and guest (host:guest). -mac must be set correctly.")
	var (
		debug         = flag.Bool("debug", false, "enable debug print")
		listenWS      = flag.Bool("listen-ws", false, "listen on a websocket port specified as argument")
		enableTLS     = flag.Bool("enable-tls", false, "enable TLS for the websocket connection")
		wsCert        = flag.String("ws-cert", "", "TLS cert for ws connection")
		wsKey         = flag.String("ws-key", "", "TLS key for ws connection")
		invoke        = flag.Bool("invoke", false, "invoke the container with NW support")
		mac           = flag.String("mac", vmMAC, "mac address assigned to the container")
		wasiAddr      = flag.String("wasi-addr", "127.0.0.1:1234", "IP address used to communicate between wasi and network stack (valid only with invoke flag)") // TODO: automatically use empty random port or unix socket
		wasmtimeCli13 = flag.Bool("wasmtime-cli-13", false, "Use old wasmtime CLI (<= 13)")
	)
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		panic("specify args")
	}
	socketAddr := args[0]
	forwards := make(map[string]string)
	for _, p := range portFlags {
		parts := strings.Split(p, ":")
		switch len(parts) {
		case 3:
			// IP:PORT1:PORT2
			forwards[strings.Join(parts[0:2], ":")] = strings.Join([]string{vmIP, parts[2]}, ":")
		case 2:
			// PORT1:PORT2
			forwards["0.0.0.0:"+parts[0]] = vmIP + ":" + parts[1]
		}
	}
	if *debug {
		log.SetOutput(os.Stderr)
	} else {
		log.SetOutput(io.Discard)
	}
	log.Printf("port mapping: %+v\n", forwards)
	config := &gvntypes.Configuration{
		Debug:             *debug,
		MTU:               1500,
		Subnet:            "192.168.127.0/24",
		GatewayIP:         gatewayIP,
		GatewayMacAddress: "5a:94:ef:e4:0c:dd",
		DHCPStaticLeases: map[string]string{
			vmIP: *mac,
		},
		Forwards: forwards,
		NAT: map[string]string{
			"192.168.127.254": "127.0.0.1",
		},
		GatewayVirtualIPs: []string{"192.168.127.254"},
		Protocol:          gvntypes.QemuProtocol,
	}
	vn, err := gvnvirtualnetwork.New(config)
	if err != nil {
		panic(err)
	}
	if *invoke {
		go func() {
			fmt.Fprintf(os.Stderr, "waiting for NW initialization\n")
			var conn net.Conn
			for i := 0; i < 10; i++ {
				time.Sleep(1 * time.Second)
				log.Printf("connecting to NW...\n")
				conn, err = net.Dial("tcp", *wasiAddr)
				if err == nil {
					break
				}
				log.Printf("failed connecting to NW: %v\n", err)
			}
			if conn == nil {
				log.Fatalf("failed to connect to vm: lasterr=%d", err)
			}
			// We register our VM network as a qemu "-netdev socket".
			if err := vn.AcceptQemu(context.TODO(), conn); err != nil {
				log.Printf("failed AcceptQemu: %v\n", err)
			}
		}()
		var cmd *exec.Cmd
		if *wasmtimeCli13 {
			cmd = exec.Command("wasmtime", append([]string{"run", "--tcplisten=" + *wasiAddr, "--env='LISTEN_FDS=1'", "--"}, args...)...)
		} else {
			cmd = exec.Command("wasmtime", append([]string{"run", "-S", "preview2=n", "-S", "tcplisten=" + *wasiAddr, "--env='LISTEN_FDS=1'", "--"}, args...)...)
		}
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			panic(err)
		}
		return
	}
	if *listenWS {
		http.Handle("/", websocket.Handler(func(ws *websocket.Conn) {
			ws.PayloadType = websocket.BinaryFrame
			if err := vn.AcceptQemu(context.TODO(), ws); err != nil {
				log.Printf("forwarding finished: %v\n", err)
			}
		}))
		if *enableTLS {
			if err := http.ListenAndServeTLS(socketAddr, *wsCert, *wsKey, nil); err != nil {
				panic(err)
			}
		} else {
			if err := http.ListenAndServe(socketAddr, nil); err != nil {
				panic(err)
			}
		}
		return
	}
	conn, err := net.Dial("tcp", socketAddr)
	if err != nil {
		panic(err)
	}
	// We register our VM network as a qemu "-netdev socket".
	if err := vn.AcceptQemu(context.TODO(), conn); err != nil {
		panic(err)
	}
}

type sliceFlags []string

func (f *sliceFlags) String() string {
	var s []string = *f
	return fmt.Sprintf("%v", s)
}

func (f *sliceFlags) Set(value string) error {
	*f = append(*f, value)
	return nil
}
