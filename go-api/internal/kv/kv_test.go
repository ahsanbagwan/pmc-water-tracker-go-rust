package kv

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func exercise(t *testing.T, k KV) {
	ctx := context.Background()
	if _, ok, _ := k.Get(ctx, "a"); ok {
		t.Fatal("unexpected key")
	}
	_ = k.Set(ctx, "a", "1", 0)
	if v, ok, _ := k.Get(ctx, "a"); !ok || v != "1" {
		t.Fatalf("get %q %v", v, ok)
	}
	n1, _ := k.Incr(ctx, "c")
	n2, _ := k.Incr(ctx, "c")
	if n1 != 1 || n2 != 2 {
		t.Fatalf("incr %d %d", n1, n2)
	}
	if ok, _ := k.SetNX(ctx, "lock", "me", time.Minute); !ok {
		t.Fatal("first SetNX should win")
	}
	if ok, _ := k.SetNX(ctx, "lock", "you", time.Minute); ok {
		t.Fatal("second SetNX should lose")
	}
	_ = k.Del(ctx, "a")
	if _, ok, _ := k.Get(ctx, "a"); ok {
		t.Fatal("del failed")
	}
}

func TestMemory(t *testing.T) {
	exercise(t, NewMemory())
}

func TestMemoryTTLAndPubSub(t *testing.T) {
	m := NewMemory()
	now := time.Now()
	m.now = func() time.Time { return now }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = m.Set(ctx, "k", "v", time.Second)
	now = now.Add(2 * time.Second)
	if _, ok, _ := m.Get(ctx, "k"); ok {
		t.Fatal("expired key still there")
	}
	ch, _ := m.Subscribe(ctx, "c")
	_ = m.Publish(ctx, "c", "hello")
	select {
	case got := <-ch:
		if got != "hello" {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("no message")
	}
}

// fakeRedis speaks just enough RESP to test the client over a real socket.
func fakeRedis(t *testing.T) (string, string) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	var mu sync.Mutex
	data := map[string]string{}
	var subs []net.Conn
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					n, _ := strconv.Atoi(strings.TrimSpace(line[1:]))
					args := make([]string, n)
					for i := range args {
						r.ReadString('\n')
						a, _ := r.ReadString('\n')
						args[i] = strings.TrimRight(a, "\r\n")
					}
					mu.Lock()
					switch strings.ToUpper(args[0]) {
					case "SET":
						if len(args) > 3 && strings.ToUpper(args[3]) == "NX" {
							if _, ok := data[args[1]]; ok {
								fmt.Fprint(c, "$-1\r\n")
								mu.Unlock()
								continue
							}
						}
						data[args[1]] = args[2]
						fmt.Fprint(c, "+OK\r\n")
					case "GET":
						if v, ok := data[args[1]]; ok {
							fmt.Fprintf(c, "$%d\r\n%s\r\n", len(v), v)
						} else {
							fmt.Fprint(c, "$-1\r\n")
						}
					case "DEL":
						delete(data, args[1])
						fmt.Fprint(c, ":1\r\n")
					case "INCR":
						v, _ := strconv.Atoi(data[args[1]])
						v++
						data[args[1]] = strconv.Itoa(v)
						fmt.Fprintf(c, ":%d\r\n", v)
					case "PEXPIRE":
						fmt.Fprint(c, ":1\r\n")
					case "SUBSCRIBE":
						subs = append(subs, c)
						fmt.Fprintf(c, "*3\r\n$9\r\nsubscribe\r\n$%d\r\n%s\r\n:1\r\n", len(args[1]), args[1])
					case "PUBLISH":
						for _, s := range subs {
							fmt.Fprintf(s, "*3\r\n$7\r\nmessage\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(args[1]), args[1], len(args[2]), args[2])
						}
						fmt.Fprintf(c, ":%d\r\n", len(subs))
					default:
						fmt.Fprint(c, "-ERR unknown\r\n")
					}
					mu.Unlock()
				}
			}(c)
		}
	}()
	host, port, _ := net.SplitHostPort(l.Addr().String())
	return host, port
}

func TestRedisClientAgainstFakeServer(t *testing.T) {
	host, port := fakeRedis(t)
	r := NewRedis(host, port, "", false)
	exercise(t, r)
}

func TestRedisPubSub(t *testing.T) {
	host, port := fakeRedis(t)
	r := NewRedis(host, port, "", false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := r.Subscribe(ctx, "updates")
	_ = r.Publish(ctx, "updates", `{"id":1}`)
	select {
	case m := <-ch:
		if m != `{"id":1}` {
			t.Fatal(m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no pubsub message")
	}
}

func TestRedisUnavailable(t *testing.T) {
	r := NewRedis("127.0.0.1", "1", "", false)
	if _, err := r.Incr(context.Background(), "x"); err == nil {
		t.Fatal("expected error when redis is down")
	}
}
