go-wrk - an HTTP benchmarking tool
==================================

go-wrk is a modern HTTP benchmarking tool capable of generating significant load when run on a single multi-core CPU. It builds on go language go routines and scheduler for behind the scenes async IO and concurrency.

It was created mostly to examine go language (http://golang.org) performance and verbosity compared to C (the language wrk was written in. See - <https://github.com/wg/wrk>).  
It turns out that it is just as good in terms of throughput! And with a lot less code.  

The majority of go-wrk is the product of one afternoon, and its quality is comparable to wrk.

Building
--------

    go install github.com/tsliwowicz/go-wrk@latest

This will download and compile go-wrk. 
   
Command line parameters (./go-wrk -help)  
	
       Usage: go-wrk <options> <url>
       Options:
        -H       Header to add to each request (you can define multiple -H flags) (Default )
        -M       HTTP method (Default GET)
        -T       Socket/request timeout in ms (Default 1000)
        -body    request body string or @filename (Default )
        -c       Number of goroutines to use (concurrent connections) (Default 10)
        -ca      CA file to verify peer against (SSL/TLS) (Default )
        -cert    CA certificate file to verify peer against (SSL/TLS) (Default )
        -d       Duration of test in seconds (Default 10)
        -f       Playback file name (Default <empty>)
        -help    Print help (Default false)
        -host    Host Header (Default )
        -http    Use HTTP/2 (Default true)
        -key     Private key file name (SSL/TLS (Default )
        -no-c    Disable Compression - Prevents sending the "Accept-Encoding: gzip" header (Default false)
        -no-ka   Disable KeepAlive - prevents re-use of TCP connections between different HTTP requests (Default false)
        -no-vr   Skip verifying SSL certificate of the server (Default false)
        -redir   Allow Redirects (Default false)
        -v       Print version details (Default false)

Basic Usage
-----------

    ./go-wrk -c 2048 -d 10 http://localhost:8080/plaintext

This runs a benchmark for 10 seconds, using 2048 go routines (connections)

Output:

    Running 10s test @ http://localhost:8080/plaintext
        2048 goroutine(s) running concurrently
    439977 requests in 10.012950719s, 52.45MB read
    Requests/sec:		43940.79
    Transfer/sec:		5.24MB
    Fastest Request:	98µs
    Avg Req Time:		46.608ms
    Slowest Request:	398.431ms
    Number of Errors:	0
    Error Counts:		map[]
    10%:			    164µs
    50%:			    2.382ms
    75%:			    3.83ms
    99%:			    5.403ms
    99.9%:			    5.488ms
    99.9999%:		    5.5ms
    99.99999%:		    5.5ms
    stddev:			    29.744ms


Benchmarking Tips
-----------------

  The machine running go-wrk must have a sufficient number of ephemeral ports
  available and closed sockets should be recycled quickly. To handle the
  initial connection burst the server's listen(2) backlog should be greater
  than the number of concurrent connections being tested.

Acknowledgements
----------------

  golang is awesome. I did not need anything but this to create go-wrk.  
  I fully credit the wrk project (https://github.com/wg/wrk) for the inspiration and even parts of this text.  
  I also used similar command line arguments format and output format.
