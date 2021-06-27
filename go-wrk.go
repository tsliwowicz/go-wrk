package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"github.com/tsliwowicz/go-wrk/loader"
	"github.com/tsliwowicz/go-wrk/util"
)

const APP_VERSION = "0.9"

//default that can be overridden from the command line
var versionFlag bool = false
var helpFlag bool = false
var duration int = 10 //seconds
var goroutines int = 2
var testUrl string
var method string = "GET"
var host string
var headerFlags util.HeaderList
var header map[string]string
var statsAggregator chan *loader.RequesterStats
var timeoutms int
var allowRedirectsFlag bool = false
var disableCompression bool
var disableKeepAlive bool
var skipVerify bool
var playbackFile string
var reqBody string
var clientCert string
var clientKey string
var caCert string
var http2 bool

func init() {
	flag.BoolVar(&versionFlag, "v", false, "Print version details")
	flag.BoolVar(&allowRedirectsFlag, "redir", false, "Allow Redirects")
	flag.BoolVar(&helpFlag, "help", false, "Print help")
	flag.BoolVar(&disableCompression, "no-c", false, "Disable Compression - Prevents sending the \"Accept-Encoding: gzip\" header")
	flag.BoolVar(&disableKeepAlive, "no-ka", false, "Disable KeepAlive - prevents re-use of TCP connections between different HTTP requests")
	flag.BoolVar(&skipVerify, "no-vr", false, "Skip verifying SSL certificate of the server")
	flag.IntVar(&goroutines, "c", 10, "Number of goroutines to use (concurrent connections)")
	flag.IntVar(&duration, "d", 10, "Duration of test in seconds")
	flag.IntVar(&timeoutms, "T", 1000, "Socket/request timeout in ms")
	flag.StringVar(&method, "M", "GET", "HTTP method")
	flag.StringVar(&host, "host", "", "Host Header")
	flag.Var(&headerFlags, "H", "Header to add to each request (you can define multiple -H flags)")
	flag.StringVar(&playbackFile, "f", "<empty>", "Playback file name")
	flag.StringVar(&reqBody, "body", "", "request body string or @filename")
	flag.StringVar(&clientCert, "cert", "", "CA certificate file to verify peer against (SSL/TLS)")
	flag.StringVar(&clientKey, "key", "", "Private key file name (SSL/TLS")
	flag.StringVar(&caCert, "ca", "", "CA file to verify peer against (SSL/TLS)")
	flag.BoolVar(&http2, "http", true, "Use HTTP/2")
}

//printDefaults a nicer format for the defaults
func printDefaults() {
	fmt.Println("Usage: go-wrk <options> <url>")
	fmt.Println("Options:")
	flag.VisitAll(func(flag *flag.Flag) {
		fmt.Println("\t-"+flag.Name, "\t", flag.Usage, "(Default "+flag.DefValue+")")
	})
}

func main() {
	//raising the limits. Some performance gains were achieved with the + goroutines (not a lot).
	runtime.GOMAXPROCS(runtime.NumCPU() + goroutines)

	statsAggregator = make(chan *loader.RequesterStats, goroutines)
	sigChan := make(chan os.Signal, 1)

	signal.Notify(sigChan, os.Interrupt)

	flag.Parse() // Scan the arguments list
	header = make(map[string]string)
	if headerFlags != nil {
		for _, hdr := range headerFlags {
			hp := strings.SplitN(hdr, ":", 2)
			header[hp[0]] = hp[1]
		}
	}

	if playbackFile != "<empty>" {
		file, err := os.Open(playbackFile) // For read access.
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer file.Close()
		url, err := ioutil.ReadAll(file)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		testUrl = string(url)
	} else {
		testUrl = flag.Arg(0)
	}

	if versionFlag {
		fmt.Println("Version:", APP_VERSION)
		return
	} else if helpFlag || len(testUrl) == 0 {
		printDefaults()
		return
	}

	fmt.Printf("Running %vs test @ %v\n  %v goroutine(s) running concurrently\n", duration, testUrl, goroutines)

	if len(reqBody) > 0 && reqBody[0] == '@' {
		bodyFilename := reqBody[1:]
		data, err := ioutil.ReadFile(bodyFilename)
		if err != nil {
			fmt.Println(fmt.Errorf("could not read file %q: %v", bodyFilename, err))
			os.Exit(1)
		}
		reqBody = string(data)
	}

	loadGen := loader.NewLoadCfg(duration, goroutines, testUrl, reqBody, method, host, header, statsAggregator, timeoutms,
		allowRedirectsFlag, disableCompression, disableKeepAlive, skipVerify, clientCert, clientKey, caCert, http2)

	for i := 0; i < goroutines; i++ {
		go loadGen.RunSingleLoadSession()
	}

	responders := 0
	aggStats := loader.RequesterStats{MinRequestTime: time.Minute}

	for responders < goroutines {
		select {
		case <-sigChan:
			loadGen.Stop()
			fmt.Printf("stopping...\n")
		case stats := <-statsAggregator:
			aggStats.NumErrs += stats.NumErrs
			aggStats.NumRequests += stats.NumRequests
			aggStats.TotRespSize += stats.TotRespSize
			aggStats.TotDuration += stats.TotDuration
			aggStats.MaxRequestTime = util.MaxDuration(aggStats.MaxRequestTime, stats.MaxRequestTime)
			aggStats.MinRequestTime = util.MinDuration(aggStats.MinRequestTime, stats.MinRequestTime)
			responders++
		}
	}

	if aggStats.NumRequests == 0 {
		fmt.Println("Error: No statistics collected / no requests found\n")
		return
	}

	avgThreadDur := aggStats.TotDuration / time.Duration(responders) //need to average the aggregated duration

	reqRate := float64(aggStats.NumRequests) / avgThreadDur.Seconds()
	avgReqTime := aggStats.TotDuration / time.Duration(aggStats.NumRequests)
	bytesRate := float64(aggStats.TotRespSize) / avgThreadDur.Seconds()
	fmt.Printf("%v requests in %v, %v read\n", aggStats.NumRequests, avgThreadDur, util.ByteSize{float64(aggStats.TotRespSize)})
	fmt.Printf("Requests/sec:\t\t%.2f\nTransfer/sec:\t\t%v\nAvg Req Time:\t\t%v\n", reqRate, util.ByteSize{bytesRate}, avgReqTime)
	fmt.Printf("Fastest Request:\t%v\n", aggStats.MinRequestTime)
	fmt.Printf("Slowest Request:\t%v\n", aggStats.MaxRequestTime)
	fmt.Printf("Number of Errors:\t%v\n", aggStats.NumErrs)
}
