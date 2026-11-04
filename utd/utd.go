package main

import (
	"fmt"
	"math"
	"net"
	"time"
)

type ProbeResult struct {
	AvgMs    float64
	JitterMs float64
	Loss     float64
	MinMs    float64
	MaxMs    float64
}

// UDP Echo服务端 —— 收到啥就原样返回啥
func startEchoServer(port int) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Echo server listen udp :%d\n", port)
	buf := make([]byte, 1024)
	go func() {
		for {
			n, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}
			_, _ = conn.WriteToUDP(buf[:n], remote) //原样回包
		}
	}()
}

// UDP探测客户端
func UdpProbe(host string, port int, count int, timeoutMs int) ProbeResult {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return ProbeResult{Loss: 1.0}
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return ProbeResult{Loss: 1.0}
	}
	defer conn.Close()

	var rttList []float64
	lost := 0
	sendBuf := []byte("probe")

	for i := 0; i < count; i++ {
		sendTime := time.Now()
		_, _ = conn.Write(sendBuf)
		conn.SetReadDeadline(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond))
		rb := make([]byte, 1024)
		n, err := conn.Read(rb)
		if err != nil || n == 0 {
			lost++
			continue
		}
		rtt := float64(time.Since(sendTime).Microseconds()) / 1000.0
		rttList = append(rttList, rtt)
	}

	if len(rttList) == 0 {
		return ProbeResult{Loss: 1.0}
	}

	var sum float64
	min, max := rttList[0], rttList[0]
	for _, v := range rttList {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	avg := sum / float64(len(rttList))

	var variance float64
	for _, v := range rttList {
		variance += (v - avg) * (v - avg)
	}
	jitter := math.Sqrt(variance / float64(len(rttList)))
	lossRate := float64(lost) / float64(count)

	return ProbeResult{
		AvgMs:    avg,
		JitterMs: jitter,
		Loss:     lossRate,
		MinMs:    min,
		MaxMs:    max,
	}
}

func main() {
	echoPort := 8888
	startEchoServer(echoPort)
	time.Sleep(200 * time.Millisecond) //等服务启动

	//探测本地Echo服务，会有正常返回
	res := UdpProbe("127.0.0.1", echoPort, 10, 500)
	fmt.Printf("Avg:%.2fms Jitter:%.2fms Loss:%.2f%% Min:%.2fms Max:%.2fms\n",
		res.AvgMs, res.JitterMs, res.Loss*100, res.MinMs, res.MaxMs)
}
