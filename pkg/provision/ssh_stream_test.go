package provision

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// Exercise the actual x/crypto stdout/stderr copy goroutines, not only a direct
// synchronizedBuffer write. This server never executes a shell or node command.
func TestRealSSHConcurrentStdoutStderr(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	done := make(chan error, 1)
	go func() {
		raw, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer raw.Close()
		conn, channels, requests, err := ssh.NewServerConn(raw, config)
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		go ssh.DiscardRequests(requests)
		newChannel := <-channels
		channel, requests, err := newChannel.Accept()
		if err != nil {
			done <- err
			return
		}
		defer channel.Close()
		for req := range requests {
			if req.Type != "exec" {
				req.Reply(false, nil)
				continue
			}
			req.Reply(true, nil)
			_, err := io.Copy(io.Discard, channel)
			if err != nil {
				done <- err
				return
			}
			var wg sync.WaitGroup
			for _, stream := range []io.Writer{channel, channel.Stderr()} {
				wg.Add(1)
				go func(w io.Writer) {
					defer wg.Done()
					for i := 0; i < 1000; i++ {
						_, _ = io.WriteString(w, "__STREAM_MARKER__\n")
					}
				}(stream)
			}
			wg.Wait()
			_, err = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
			done <- err
			return
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := (&RealSSH{}).Run(ctx, NodeSpec{ID: "fixture", Host: "127.0.0.1", Port: port}, pemKey, "fixture-only")
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "__STREAM_MARKER__\n") != 2000 {
		t.Fatal("real SSH stdout/stderr copies lost or corrupted output")
	}
}
