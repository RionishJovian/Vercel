package cache

import (
	"bufio"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mega-magic/internal/config"
)

// This speaks enough RESP for New's real client handshake, then leaves the
// readiness PING unanswered. Boolean probe stubs cannot detect socket deadlines.
func TestReadinessBoundsUnresponsiveRedisSocket(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var hang atomic.Bool
	var workers sync.WaitGroup
	acceptDone := make(chan struct{})
	defer func() { close(stop); ln.Close(); workers.Wait() }()
	go func() {
		defer close(acceptDone)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer conn.Close()
				go func() { <-stop; conn.Close() }()
				r := bufio.NewReader(conn)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					count, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "*")))
					if err != nil {
						return
					}
					var command string
					for i := 0; i < count; i++ {
						line, err = r.ReadString('\n')
						if err != nil {
							return
						}
						n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "$")))
						if err != nil || n < 0 || n > 4096 {
							return
						}
						b := make([]byte, n+2)
						if _, err = io.ReadFull(r, b); err != nil {
							return
						}
						if i == 0 {
							command = strings.ToLower(string(b[:n]))
						}
					}
					switch command {
					case "hello":
						_, err = io.WriteString(conn, "-ERR unknown command 'hello'\r\n")
					case "ping":
						if hang.Load() {
							<-stop
							return
						}
						_, err = io.WriteString(conn, "+PONG\r\n")
					default:
						_, err = io.WriteString(conn, "+OK\r\n")
					}
					if err != nil {
						return
					}
				}
			}()
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	c, err := New(config.RedisConfig{Host: "127.0.0.1", Port: addr.Port, DialTimeout: time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: time.Second, PoolSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	hang.Store(true)
	result := make(chan bool, 1)
	start := time.Now()
	go func() { result <- c.IsConnected() }()
	select {
	case healthy := <-result:
		if healthy {
			t.Fatal("unresponsive Redis reported healthy")
		}
		if time.Since(start) > 3*time.Second {
			t.Fatal("readiness exceeded its two-second I/O budget")
		}
	case <-time.After(3500 * time.Millisecond):
		t.Fatal("readiness context did not bound the Redis socket read")
	}
}
