package main

import (
	"flag"
	"fmt"
	"runtime"
)

/*
Usage: wrk <options> <url>
  Options:
    -c, --connections <N>  Connections to keep open
    -d, --duration    <T>  Duration of test
    -t, --threads     <N>  Number of threads to use

    -H, --header      <H>  Add header to request
    -M, --method      <M>  HTTP method
        --body        <B>  Request body
        --latency          Print latency statistics
        --timeout     <T>  Socket/request timeout
    -v, --version          Print version details
*/

const APP_VERSION = "0.1"

var versionFlag bool = false
var helpFlag bool = false
var connections int = 1
var duration int = 10 //seconds
var threads int = 1
var url string

func init() {
	flag.BoolVar(&versionFlag, "v", false, "Print version details")
	flag.BoolVar(&helpFlag, "help", false, "Print help")
	flag.IntVar(&threads, "t", 1, "Number of goroutines to use")
	flag.IntVar(&duration, "d", 1, "Duration of test")
}

func printDefaults() {
	fmt.Println("Usage: go-wrk <options> <url>")
	fmt.Println("Options:")
	flag.VisitAll(func (flag *flag.Flag) {
		fmt.Println("\t-"+flag.Name, "\t", flag.Usage, "(Default "+flag.DefValue+")")
	})
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	flag.Parse() // Scan the arguments list
	
	url = flag.Arg(0)

	if versionFlag {
		fmt.Println("Version:", APP_VERSION)
		return
	} else if helpFlag || len(url) == 0 {
		printDefaults()
		return
	}
	
}
