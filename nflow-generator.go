// Run using:
// go run nflow-generator.go nflow_logging.go nflow_payload.go  -t 172.16.86.138 -p 9995
// Or:
// go build
// ./nflow-generator -t <ip> -p <port>
package main

import (
	"fmt"
	"github.com/jessevdk/go-flags"
	"math/rand"
	"net"
	"os"
	"time"
)

type Proto int

const (
	FTP Proto = iota + 1
	SSH
	DNS
	HTTP
	HTTPS
	NTP
	SNMP
	IMAPS
	MYSQL
	HTTPS_ALT
	P2P
	BITTORRENT
)

var opts struct {
	CollectorIP   string `short:"t" long:"target" description:"target ip address of the netflow collector"`
	CollectorPort string `short:"p" long:"port" description:"port number of the target netflow collector"`
	SpikeProto    string `short:"s" long:"spike" description:"run a second thread generating a spike for the specified protocol"`
	FalseIndex    bool   `short:"f" long:"false-index" description:"generate false SNMP interface indexes, otherwise set to 0"`
	Duration      int    `short:"d" long:"duration" description:"total duration time in milliseconds" default:"10000"`
	Times         int    `short:"" long:"times" description:"how many times" default:"0"`
	Interval      int    `short:"u" long:"interval" description:"interval between each batch in milliseconds" default:"1000"`
	FlowCount     int    `short:"c" long:"flow-count" description:"flow per interval" default:"100"`
	BatchSize     int    `short:"b" long:"batch-size" description:"batch size" default:"30"`
	EngineId      uint8  `short:"e" long:"engine-id" description:"engine id" default:"0"`
	Help          bool   `short:"h" long:"help" description:"show nflow-generator help"`
}

func main() {

	_, err := flags.Parse(&opts)
	if err != nil {
		showUsage()
		os.Exit(1)
	}
	if opts.Help == true {
		showUsage()
		os.Exit(1)
	}
	if opts.CollectorIP == "" || opts.CollectorPort == "" {
		showUsage()
		os.Exit(1)
	}
	collector := opts.CollectorIP + ":" + opts.CollectorPort
	udpAddr, err := net.ResolveUDPAddr("udp", collector)
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		log.Fatal("Error connecting to the target collector: ", err)
	}
	log.Infof("sending netflow data to a collector ip: %s and port: %s. \n"+
		"Use ctrl^c to terminate the app.", opts.CollectorIP, opts.CollectorPort)

	ticker := time.NewTicker(time.Duration(opts.Interval) * time.Millisecond)
	done := make(chan bool)

	start := time.Now()

	go func() {
		total := loop(done, ticker, 0, 1000, 30, conn, start, opts.EngineId)
		total = loop(done, ticker, total, 50000, 60, conn, start, opts.EngineId)
		total = loop(done, ticker, total, 1000, 30, conn, start, opts.EngineId)

		//times := opts.Times - 1
		//
		//total := loop(done, ticker, 0, opts.FlowCount, times, conn, start)
		//
		//times = 20
		//flowCount := opts.FlowCount / 20
		//loop(done, ticker, total, flowCount, times, conn, start)

	}()

	if opts.Duration == 0 {
		// block forever
		select {}
	} else {
		time.Sleep(time.Duration(opts.Duration) * time.Millisecond)
		ticker.Stop()
		done <- true
		fmt.Println("Ticker stopped")
	}
}

func loop(done chan bool, ticker *time.Ticker, prevTotal int, flowCount int, times int, conn *net.UDPConn, start time.Time, engineId uint8) int {
	total := prevTotal
	count := 0
	for {
		select {
		case <-done:
			return total
		case t := <-ticker.C:
			if count >= times {
				return total
			}
			count += 1
			fmt.Printf("Tick at %s Count=%d\n", t, count)

			iterations := flowCount / opts.BatchSize
			remainder := flowCount % opts.BatchSize
			var packets []Netflow
			for i := 0; i < iterations; i++ {
				generated := GenerateNetflow(opts.BatchSize, engineId)
				if count == 3 && i == 0 {
					continue
				}
				packets = append(packets, generated)
				total += opts.BatchSize
			}
			// go build && ./nflow-generator -t 127.0.0.1 -p 9995 --duration 0 --interval 10000 --flow-count 2 --times 5 --batch-size 2
			if count == 2 {
				p2 := packets[len(packets)-2]
				p1 := packets[len(packets)-1]
				packets[len(packets)-1] = p2
				packets[len(packets)-2] = p1
			}
			for _, packet := range packets {
				fmt.Printf("\t[Packet] Seq=%d Count=%d\n", packet.Header.FlowSequence, packet.Header.FlowCount)
				buffer := BuildNFlowPayload(packet)
				_, err := conn.Write(buffer.Bytes())
				if err != nil {
					log.Fatal("Error connecting to the target collector: ", err)
				}
			}

			if remainder > 0 {
				total += remainder
				generate(conn, remainder, engineId)
			}

			//total += opts.FlowCount
			flowPerSec := float64(total) / (float64(time.Since(start).Microseconds() / 1000000))
			fmt.Printf("\tCumulative flows send %d, flows per sec: %.2f\n", total, flowPerSec)
		}
	}
	return 0
}

func generate(conn *net.UDPConn, batchSize int, engineId uint8) {
	data := GenerateNetflow(batchSize, engineId)
	buffer := BuildNFlowPayload(data)
	_, err := conn.Write(buffer.Bytes())
	if err != nil {
		log.Fatal("Error connecting to the target collector: ", err)
	}
}

func randomNum(min, max int) int {
	return rand.Intn(max-min) + min
}

func showUsage() {
	var usage string
	usage = `
Usage:
  main [OPTIONS] [collector IP address] [collector port number]

  Send mock Netflow version 5 data to designated collector IP & port.
  Time stamps in all datagrams are set to UTC.

Application Options:
  -t, --target= target ip address of the netflow collector
  -p, --port=   port number of the target netflow collector
  -s, --spike run a second thread generating a spike for the specified protocol
    protocol options are as follows:
        ftp - generates tcp/21
        ssh  - generates tcp/22
        dns - generates udp/54
        http - generates tcp/80
        https - generates tcp/443
        ntp - generates udp/123
        snmp - generates ufp/161
        imaps - generates tcp/993
        mysql - generates tcp/3306
        https_alt - generates tcp/8080
        p2p - generates udp/6681
        bittorrent - generates udp/6682
  -f, --false-index generate a false snmp index values of 1 or 2. The default is 0. (Optional)
  -c, --flow-count set the number of flows to generate in each iteration. The default is 16. (Optional)

Example Usage:

    -first build from source (one time)
    go build   

    -generate default flows to device 172.16.86.138, port 9995
    ./nflow-generator -t 172.16.86.138 -p 9995 

    -generate default flows along with a spike in the specified protocol:
    ./nflow-generator -t 172.16.86.138 -p 9995 -s ssh

    -generate default flows with "false index" settings for snmp interfaces 
    ./nflow-generator -t 172.16.86.138 -p 9995 -f

    -generate default flows with up to 256 flows
    ./nflow-generator -c 128 -t 172.16.86.138 -p 9995

Help Options:
  -h, --help    Show this help message
  `
	fmt.Print(usage)
}
