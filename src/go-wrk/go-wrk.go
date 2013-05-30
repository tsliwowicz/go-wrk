package main

import (
	//"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"runtime"
	"time"
)

type RequesterStats struct {
	totBodySize int64
	totDuration time.Duration
	numRequests int
	numErrs     int
}

type RedirectError struct {
	msg string
}

func (self *RedirectError) Error() string {
	return self.msg
}

func NewRedirectError(message string) *RedirectError {
	rt := RedirectError{msg: message}
	return &rt
}

type ByteSize struct {
	size float64
}

func (self ByteSize) String() string {
	var rt float64
	var suffix string
	const (
		Byte  = 1
		KByte = Byte * 1024
		MByte = KByte * 1024
		GByte = MByte * 1024
	)

	if self.size > GByte {
		rt = self.size / GByte
		suffix = "GB"
	} else if self.size > MByte {
		rt = self.size / MByte
		suffix = "MB"
	} else if self.size > KByte {
		rt = self.size / KByte
		suffix = "KB"
	} else {
		rt = self.size
		suffix = "bytes"
	}

	srt := fmt.Sprintf("%.2f%v", rt, suffix)

	return srt
}

const APP_VERSION = "0.1"

var versionFlag bool = false
var helpFlag bool = false
var duration int = 10 //seconds
var threads int = 2
var testUrl string
var method string = "GET"
var statsAggregator chan *RequesterStats
var timeoutms int
var allowRedirectsFlag bool = false

func init() {
	flag.BoolVar(&versionFlag, "v", false, "Print version details")
	flag.BoolVar(&allowRedirectsFlag, "redir", false, "Allow Redirects")
	flag.BoolVar(&helpFlag, "help", false, "Print help")
	flag.IntVar(&threads, "t", 2, "Number of goroutines to use (concurrent requests)")
	flag.IntVar(&duration, "d", 10, "Duration of test")
	flag.IntVar(&timeoutms, "T", 1000, "Socket/request timeout in ms")
	flag.StringVar(&method, "M", "GET", "HTTP method")
}

func printDefaults() {
	fmt.Println("Usage: go-wrk <options> <url>")
	fmt.Println("Options:")
	flag.VisitAll(func(flag *flag.Flag) {
		fmt.Println("\t-"+flag.Name, "\t", flag.Usage, "(Default "+flag.DefValue+")")
	})
}

func estimateHeadersSize(headers http.Header) (result int64) {
	result = 0

	for k, v := range headers {
		result += int64(len(k)+len(": \r\n")) 
		for _, s := range v {
			result += int64(len(s))
		}
	}
	
	result += int64(len("\r\n")) 
	
	return result
}

func DoRequest(httpClient *http.Client) (respBodySize int, duration time.Duration) {
	respBodySize = -1
	duration = -1
	req, err := http.NewRequest(method, testUrl, nil)

	req.Header.Add("User-Agent", "go-wrk, version "+APP_VERSION)
	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
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
			respBodySize = len(body) + int(estimateHeadersSize(resp.Header))
		}
	} else if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusTemporaryRedirect {
		duration = time.Since(start)
		respBodySize = int(resp.ContentLength) + int(estimateHeadersSize(resp.Header))
	}

	return
}

func Requester() {
	stats := &RequesterStats{}
	start := time.Now()
	var httpClient *http.Client

	if allowRedirectsFlag {
		httpClient = &http.Client{}
	} else {
		httpClient = &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return NewRedirectError("redirection not allowed") }}
	}

	httpClient.Transport = &http.Transport{ResponseHeaderTimeout: time.Millisecond * time.Duration(timeoutms)}

	for time.Since(start).Seconds() <= float64(duration) {
		respBodySize, reqDur := DoRequest(httpClient)
		if respBodySize > 0 {
			stats.totBodySize += int64(respBodySize)
			stats.totDuration += reqDur
			stats.numRequests++
		} else {
			stats.numErrs++
		}
	}
	statsAggregator <- stats
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU() + threads)

	statsAggregator = make(chan *RequesterStats, threads)

	flag.Parse() // Scan the arguments list

	testUrl = flag.Arg(0)

	if versionFlag {
		fmt.Println("Version:", APP_VERSION)
		return
	} else if helpFlag || len(testUrl) == 0 {
		printDefaults()
		return
	}

	fmt.Printf("Running %vs test @ %v\n  %v goroutine(s)\n", duration, testUrl, threads)

	for i := 0; i < threads; i++ {
		go Requester()
	}

	responders := 0
	aggStats := RequesterStats{}

	for stats := range statsAggregator {
		aggStats.numErrs += stats.numErrs
		aggStats.numRequests += stats.numRequests
		aggStats.totBodySize += stats.totBodySize
		aggStats.totDuration += stats.totDuration
		responders++
		if responders == threads {
			break
		}
	}

	aggStats.totDuration /= time.Duration(responders)

	reqRate := float64(aggStats.numRequests) / aggStats.totDuration.Seconds()
	bytesRate := float64(aggStats.totBodySize) / aggStats.totDuration.Seconds()
	fmt.Printf("%v requests in %v, %v read\n", aggStats.numRequests, aggStats.totDuration, ByteSize{float64(aggStats.totBodySize)})
	fmt.Printf("Requests/sec:\t%.2f\nTransfer/sec:\t%v\nnum errors %v\n", reqRate, ByteSize{bytesRate}, aggStats.numErrs)

	fmt.Println("Done")

}
