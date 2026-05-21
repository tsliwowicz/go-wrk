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

	histo "github.com/HdrHistogram/hdrhistogram-go"
	"github.com/tsliwowicz/go-wrk/loader"
	"github.com/tsliwowicz/go-wrk/util"
)

const APP_VERSION = "0.10"

// default that can be overridden from the command line
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
var cpus int = 0

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
	flag.IntVar(&cpus, "cpus", 0, "Number of cpus, i.e. GOMAXPROCS. 0 = system default.")
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

// printDefaults a nicer format for the defaults
func printDefaults() {
	fmt.Println("Usage: go-wrk <options> <url>")
	fmt.Println("Options:")
	flag.VisitAll(func(flag *flag.Flag) {
		fmt.Println("\t-"+flag.Name, "\t", flag.Usage, "(Default "+flag.DefValue+")")
	})
}

func mapToString(m map[string]int) string {
	s := make([]string, 0, len(m))
	for k, v := range m {
		s = append(s, fmt.Sprint(k, "=", v))
	}
	return strings.Join(s, ",")
}

func main() {

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

	if cpus > 0 {
		runtime.GOMAXPROCS(cpus)
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

	start := time.Now()

	for i := 0; i < goroutines; i++ {
		go loadGen.RunSingleLoadSession()
	}

	responders := 0
	aggStats := loader.RequesterStats{ErrMap: make(map[string]int), Histogram: histo.New(1, int64(duration*1000000), 4)}

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
			responders++
			for k, v := range stats.ErrMap {
				aggStats.ErrMap[k] += v
			}
			aggStats.Histogram.Merge(stats.Histogram)
		}
	}

	duration := time.Now().Sub(start)

	if aggStats.NumRequests == 0 {
		fmt.Println("Error: No statistics collected / no requests found")
		fmt.Printf("Number of Errors:\t%v\n", aggStats.NumErrs)
		if aggStats.NumErrs > 0 {
			fmt.Printf("Error Counts:\t\t%v\n", mapToString(aggStats.ErrMap))
		}
		return
	}

	avgThreadDur := aggStats.TotDuration / time.Duration(responders) //need to average the aggregated duration

	reqRate := float64(aggStats.NumRequests) / avgThreadDur.Seconds()
	bytesRate := float64(aggStats.TotRespSize) / avgThreadDur.Seconds()

	overallReqRate := float64(aggStats.NumRequests) / duration.Seconds()
	overallBytesRate := float64(aggStats.TotRespSize) / duration.Seconds()

	fmt.Printf("%v requests in %v, %v read\n", aggStats.NumRequests, avgThreadDur, util.ByteSize{float64(aggStats.TotRespSize)})
	fmt.Printf("Requests/sec:\t\t%.2f\nTransfer/sec:\t\t%v\n", reqRate, util.ByteSize{bytesRate})
	fmt.Printf("Overall Requests/sec:\t%.2f\nOverall Transfer/sec:\t%v\n", overallReqRate, util.ByteSize{overallBytesRate})
	fmt.Printf("Fastest Request:\t%v\n", toDuration(aggStats.Histogram.Min()))
	fmt.Printf("Avg Req Time:\t\t%v\n", toDuration(int64(aggStats.Histogram.Mean())))
	fmt.Printf("Slowest Request:\t%v\n", toDuration(aggStats.Histogram.Max()))
	fmt.Printf("Number of Errors:\t%v\n", aggStats.NumErrs)
	if aggStats.NumErrs > 0 {
		fmt.Printf("Error Counts:\t\t%v\n", mapToString(aggStats.ErrMap))
	}
	fmt.Printf("10%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(10)))
	fmt.Printf("50%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(50)))
	fmt.Printf("75%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(75)))
	fmt.Printf("99%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(99)))
	fmt.Printf("99.9%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(99.9)))
	fmt.Printf("99.9999%%:\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(99.9999)))
	fmt.Printf("99.99999%%:\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(99.99999)))
	fmt.Printf("stddev:\t\t\t%v\n", toDuration(int64(aggStats.Histogram.StdDev())))
	// aggStats.Histogram.PercentilesPrint(os.Stdout,1,1)
}

func toDuration(usecs int64) time.Duration {
	return time.Duration(usecs * 1000)
}
