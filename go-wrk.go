package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"time"
	"github.com/tsliwowicz/go-wrk/util"
)

// RequesterStats used for colelcting aggregate statistics
type RequesterStats struct {
	totRespSize    int64
	totDuration    time.Duration
	minRequestTime time.Duration
	maxRequestTime time.Duration
	numRequests    int
	numErrs        int
}


const APP_VERSION = "0.1"

//default that can be overridden from the command line
var versionFlag bool = false
var helpFlag bool = false
var duration int = 10 //seconds
var goroutines int = 2
var testUrl string
var method string = "GET"
var statsAggregator chan *RequesterStats
var timeoutms int
var allowRedirectsFlag bool = false
var interrupted int32 = 0
var disableCompression bool
var disableKeepAlive bool

func init() {
	flag.BoolVar(&versionFlag, "v", false, "Print version details")
	flag.BoolVar(&allowRedirectsFlag, "redir", false, "Allow Redirects")
	flag.BoolVar(&helpFlag, "help", false, "Print help")
	flag.BoolVar(&disableCompression, "no-c", false, "Disable Compression - Prevents sending the \"Accept-Encoding: gzip\" header")
	flag.BoolVar(&disableKeepAlive, "no-ka", false, "Disable KeepAlive - prevents re-use of TCP connections between different HTTP requests")
	flag.IntVar(&goroutines, "c", 10, "Number of goroutines to use (concurrent connections)")
	flag.IntVar(&duration, "d", 10, "Duration of test in seconds")
	flag.IntVar(&timeoutms, "T", 1000, "Socket/request timeout in ms")
	flag.StringVar(&method, "M", "GET", "HTTP method")
}

//printDefaults a nicer format for the defaults
func printDefaults() {
	fmt.Println("Usage: go-wrk <options> <url>")
	fmt.Println("Options:")
	flag.VisitAll(func(flag *flag.Flag) {
		fmt.Println("\t-"+flag.Name, "\t", flag.Usage, "(Default "+flag.DefValue+")")
	})
}

//estimateHeadersSize had to create this because headers size was not counted
func estimateHeadersSize(headers http.Header) (result int64) {
	result = 0

	for k, v := range headers {
		result += int64(len(k) + len(": \r\n"))
		for _, s := range v {
			result += int64(len(s))
		}
	}

	result += int64(len("\r\n"))

	return result
}

//DoRequest single request implementation. Returns the size of the response and its duration
//On error - returns -1 on both
func DoRequest(httpClient *http.Client) (respSize int, duration time.Duration) {
	respSize = -1
	duration = -1
	req, err := http.NewRequest(method, testUrl, nil)

	req.Header.Add("User-Agent", "go-wrk, version "+APP_VERSION)
	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		//this is a bit weird. When redirection is prevented, a url.Error is retuned. This creates an issue to distinguish
		//between an invalid URL that was provided and and redirection error.
		rr, ok := err.(*url.Error)
		if !ok {
			fmt.Println("An error occured doing request", err, rr)
			return
		}
	}
	if resp == nil {
		return
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()
	if resp.StatusCode == http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("An error occured reading body", err)
		} else {
			duration = time.Since(start)
			respSize = len(body) + int(estimateHeadersSize(resp.Header))
		}
	} else if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusTemporaryRedirect {
		duration = time.Since(start)
		respSize = int(resp.ContentLength) + int(estimateHeadersSize(resp.Header))
	}

	return
}

//Requester a go function for repeatedly making requests and aggregating statistics as long as required
//When it is done, it sends the results using the statsAggregator channel
func Requester() {
	stats := &RequesterStats{minRequestTime: time.Minute}
	start := time.Now()
	var httpClient *http.Client

	if allowRedirectsFlag {
		httpClient = &http.Client{}
	} else {
		//returning an error when trying to redirect. This prevents the redirection from happening.
		httpClient = &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return util.NewRedirectError("redirection not allowed") }}
	}

	//overriding the default parameters
	httpClient.Transport = &http.Transport{
		DisableCompression:    disableCompression,
		DisableKeepAlives:     disableKeepAlive,
		ResponseHeaderTimeout: time.Millisecond * time.Duration(timeoutms),
	}

	for time.Since(start).Seconds() <= float64(duration) && atomic.LoadInt32(&interrupted) == 0 {
		respSize, reqDur := DoRequest(httpClient)
		if respSize > 0 {
			stats.totRespSize += int64(respSize)
			stats.totDuration += reqDur
			stats.maxRequestTime = util.MaxDuration(reqDur, stats.maxRequestTime)			
			stats.minRequestTime = util.MinDuration(reqDur, stats.minRequestTime)
			stats.numRequests++
		} else {
			stats.numErrs++
		}
	}
	statsAggregator <- stats
}

func main() {
	//raising the limits. Some performance gains were achieved with the + goroutines (not a lot).
	runtime.GOMAXPROCS(runtime.NumCPU() + goroutines)

	statsAggregator = make(chan *RequesterStats, goroutines)
	sigChan := make(chan os.Signal, 1)

	signal.Notify(sigChan, os.Interrupt)

	flag.Parse() // Scan the arguments list

	testUrl = flag.Arg(0)

	if versionFlag {
		fmt.Println("Version:", APP_VERSION)
		return
	} else if helpFlag || len(testUrl) == 0 {
		printDefaults()
		return
	}

	fmt.Printf("Running %vs test @ %v\n  %v goroutine(s) running concurrently\n", duration, testUrl, goroutines)

	for i := 0; i < goroutines; i++ {
		go Requester()
	}

	responders := 0
	aggStats := RequesterStats{minRequestTime: time.Minute}

	for responders < goroutines {
		select {
		case <-sigChan:
			atomic.StoreInt32(&interrupted, 1)
			fmt.Printf("stopping...\n")
		case stats := <-statsAggregator:
			aggStats.numErrs += stats.numErrs
			aggStats.numRequests += stats.numRequests
			aggStats.totRespSize += stats.totRespSize
			aggStats.totDuration += stats.totDuration
			aggStats.maxRequestTime = util.MaxDuration(aggStats.maxRequestTime, stats.maxRequestTime)
			aggStats.minRequestTime = util.MinDuration(aggStats.minRequestTime, stats.minRequestTime)
			responders++
		}
	}
	
	if aggStats.numRequests == 0 {
		fmt.Println("Error: No statistics collected / no requests found\n")
		return
	}
	

	totThreadDur := aggStats.totDuration / time.Duration(responders) //need to average the aggregated duration

	reqRate := float64(aggStats.numRequests) / totThreadDur.Seconds()
	avgReqTime := aggStats.totDuration / time.Duration(aggStats.numRequests)
	bytesRate := float64(aggStats.totRespSize) / totThreadDur.Seconds()
	fmt.Printf("%v requests in %v, %v read\n", aggStats.numRequests, totThreadDur, util.ByteSize{float64(aggStats.totRespSize)})
	fmt.Printf("Requests/sec:\t\t%.2f\nTransfer/sec:\t\t%v\nAvg Req Time:\t\t%v\n", reqRate, util.ByteSize{bytesRate}, avgReqTime)
	fmt.Printf("Fastest Request:\t%v\n", aggStats.minRequestTime)
	fmt.Printf("Slowest Request:\t%v\n", aggStats.maxRequestTime)	
	fmt.Printf("Number of Errors:\t%v\n", aggStats.numErrs)
	
}
