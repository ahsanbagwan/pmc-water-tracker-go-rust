package kv

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Redis is a small RESP2 client (no third-party dependency): enough for GET/SET/DEL/INCR/
// EXPIRE/SET NX PX/PUBLISH/SUBSCRIBE, with a tiny connection pool and an optional TLS/AUTH.
type Redis struct {
	addr     string
	password string
	useTLS   bool
	pool     chan *rconn
	timeout  time.Duration
}

type rconn struct {
	c net.Conn
	r *bufio.Reader
}

func NewRedis(host, port, password string, useTLS bool) *Redis {
	return &Redis{addr: net.JoinHostPort(host, port), password: password, useTLS: useTLS,
		pool: make(chan *rconn, 8), timeout: 2 * time.Second}
}

func (r *Redis) dial(ctx context.Context) (*rconn, error) {
	d := net.Dialer{Timeout: r.timeout}
	var c net.Conn
	var err error
	if r.useTLS {
		c, err = tls.DialWithDialer(&d, "tcp", r.addr, &tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		c, err = d.DialContext(ctx, "tcp", r.addr)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	rc := &rconn{c: c, r: bufio.NewReader(c)}
	if r.password != "" {
		if _, err := rc.do(r.timeout, "AUTH", r.password); err != nil {
			c.Close()
			return nil, err
		}
	}
	return rc, nil
}

func (rc *rconn) do(timeout time.Duration, args ...string) (any, error) {
	_ = rc.c.SetDeadline(time.Now().Add(timeout))
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := io.WriteString(rc.c, b.String()); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return readReply(rc.r)
}

func readReply(r *bufio.Reader) (any, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return nil, fmt.Errorf("empty reply")
	}
	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return nil, fmt.Errorf("redis: %s", line[1:])
	case ':':
		return strconv.ParseInt(line[1:], 10, 64)
	case '$':
		n, _ := strconv.Atoi(line[1:])
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		return string(buf[:n]), nil
	case '*':
		n, _ := strconv.Atoi(line[1:])
		if n < 0 {
			return nil, nil
		}
		out := make([]any, n)
		for i := range out {
			if out[i], err = readReply(r); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("bad reply %q", line)
}

func (r *Redis) cmd(ctx context.Context, args ...string) (any, error) {
	var rc *rconn
	select {
	case rc = <-r.pool:
	default:
		var err error
		if rc, err = r.dial(ctx); err != nil {
			return nil, err
		}
	}
	reply, err := rc.do(r.timeout, args...)
	if err != nil && strings.HasPrefix(err.Error(), "redis:") { // server error, conn still good
		r.release(rc)
		return nil, err
	}
	if err != nil {
		rc.c.Close()
		return nil, err
	}
	r.release(rc)
	return reply, nil
}

func (r *Redis) release(rc *rconn) {
	select {
	case r.pool <- rc:
	default:
		rc.c.Close()
	}
}

func (r *Redis) Get(ctx context.Context, k string) (string, bool, error) {
	v, err := r.cmd(ctx, "GET", k)
	if err != nil || v == nil {
		return "", false, err
	}
	return v.(string), true, nil
}

func ms(d time.Duration) string { return strconv.FormatInt(d.Milliseconds(), 10) }

func (r *Redis) Set(ctx context.Context, k, v string, ttl time.Duration) error {
	args := []string{"SET", k, v}
	if ttl > 0 {
		args = append(args, "PX", ms(ttl))
	}
	_, err := r.cmd(ctx, args...)
	return err
}

func (r *Redis) Del(ctx context.Context, keys ...string) error {
	_, err := r.cmd(ctx, append([]string{"DEL"}, keys...)...)
	return err
}

func (r *Redis) Incr(ctx context.Context, k string) (int64, error) {
	v, err := r.cmd(ctx, "INCR", k)
	if err != nil {
		return 0, err
	}
	return v.(int64), nil
}

func (r *Redis) Expire(ctx context.Context, k string, ttl time.Duration) error {
	_, err := r.cmd(ctx, "PEXPIRE", k, ms(ttl))
	return err
}

func (r *Redis) SetNX(ctx context.Context, k, v string, ttl time.Duration) (bool, error) {
	reply, err := r.cmd(ctx, "SET", k, v, "NX", "PX", ms(ttl))
	return reply != nil, err
}

func (r *Redis) Publish(ctx context.Context, ch, msg string) error {
	_, err := r.cmd(ctx, "PUBLISH", ch, msg)
	return err
}

// Subscribe uses a dedicated connection and reconnects with backoff until ctx is cancelled.
func (r *Redis) Subscribe(ctx context.Context, ch string) (<-chan string, error) {
	out := make(chan string, 64)
	var once sync.Once
	first := make(chan error, 1)
	go func() {
		defer close(out)
		backoff := 500 * time.Millisecond
		for ctx.Err() == nil {
			rc, err := r.dial(ctx)
			if err == nil {
				_ = rc.c.SetDeadline(time.Time{})
				_, err = rc.do(r.timeout, "SUBSCRIBE", ch)
			}
			once.Do(func() { first <- err })
			if err != nil {
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
				}
				if backoff < 10*time.Second {
					backoff *= 2
				}
				continue
			}
			backoff = 500 * time.Millisecond
			go func() { <-ctx.Done(); rc.c.Close() }()
			for {
				_ = rc.c.SetReadDeadline(time.Time{})
				msg, err := readReply(rc.r)
				if err != nil {
					rc.c.Close()
					break // reconnect
				}
				if arr, ok := msg.([]any); ok && len(arr) == 3 && arr[0] == "message" {
					select {
					case out <- arr[2].(string):
					default:
					}
				}
			}
		}
	}()
	select {
	case err := <-first:
		if err != nil {
			return out, nil // keep retrying in the background; Redis being down must not block startup
		}
	case <-time.After(r.timeout):
	}
	return out, nil
}
