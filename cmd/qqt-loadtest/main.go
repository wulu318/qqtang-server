package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"qqtang/internal/accountauth"
)

type result struct {
	latency time.Duration
	err     error
}

func readFrame(conn net.Conn) ([]byte, error) {
	var prefix [4]byte
	if _, err := io.ReadFull(conn, prefix[:]); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(prefix[:])
	if length < 4 || length > 64*1024 {
		return nil, fmt.Errorf("invalid frame length %d", length)
	}
	frame := make([]byte, length)
	copy(frame, prefix[:])
	if _, err := io.ReadFull(conn, frame[4:]); err != nil {
		return nil, err
	}
	return frame, nil
}

func handshake(address string, uin uint32, password string) (time.Duration, error) {
	started := time.Now()
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	hello, err := accountauth.Marshal(accountauth.Message{Type: accountauth.MessageHello, UIN: uin})
	if err != nil {
		return 0, err
	}
	if _, err = conn.Write(hello); err != nil {
		return 0, err
	}
	frame, err := readFrame(conn)
	if err != nil {
		return 0, err
	}
	challenge, err := accountauth.Unmarshal(frame)
	if err != nil {
		return 0, err
	}
	if challenge.Type != accountauth.MessageChallenge {
		return 0, fmt.Errorf("unexpected message %d", challenge.Type)
	}
	proof, err := accountauth.ClientProof([]byte(password), challenge.Salt, challenge.Iterations, challenge.Nonce, uin)
	if err != nil {
		return 0, err
	}
	message, err := accountauth.Marshal(accountauth.Message{Type: accountauth.MessageProof, UIN: uin, Proof: proof})
	if err != nil {
		return 0, err
	}
	if _, err = conn.Write(message); err != nil {
		return 0, err
	}
	frame, err = readFrame(conn)
	if err != nil {
		return 0, err
	}
	final, err := accountauth.Unmarshal(frame)
	if err != nil {
		return 0, err
	}
	if !final.Accepted {
		return 0, fmt.Errorf("rejected: %s", final.Reason)
	}
	return time.Since(started), nil
}

func main() {
	address := flag.String("addr", "127.0.0.1:18000", "server address")
	count := flag.Int("n", 50, "total handshakes")
	concurrency := flag.Int("c", 10, "concurrent workers")
	password := flag.String("pass", "123456", "account password")
	flag.Parse()

	// Two seeded accounts; alternate so the test does not rely on one profile.
	uins := []uint32{1000001, 1000002}

	var next int64 = -1
	var reported int64
	results := make(chan result, *count)
	started := time.Now()
	var wg sync.WaitGroup
	workers := *concurrency
	if workers > *count {
		workers = *count
	}
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				index := int(atomic.AddInt64(&next, 1))
				if index >= *count {
					return
				}
				latency, err := handshake(*address, uins[index%len(uins)], *password)
				if err != nil && atomic.AddInt64(&reported, 1) <= 3 {
					fmt.Fprintln(os.Stderr, "sample error:", err)
				}
				results <- result{latency: latency, err: err}
			}
		}()
	}
	wg.Wait()
	close(results)
	elapsed := time.Since(started)

	var latencies []time.Duration
	failures := 0
	for res := range results {
		if res.err != nil {
			failures++
			continue
		}
		latencies = append(latencies, res.latency)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	percentile := func(p float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		index := int(float64(len(latencies)-1) * p)
		return latencies[index]
	}
	fmt.Printf("total=%d ok=%d fail=%d elapsed=%s rps=%.1f\n", *count, len(latencies), failures, elapsed.Round(time.Millisecond), float64(*count)/elapsed.Seconds())
	fmt.Printf("p50=%s p95=%s p99=%s max=%s\n", percentile(0.5), percentile(0.95), percentile(0.99), percentile(1))
	if failures > 0 {
		os.Exit(1)
	}
}
