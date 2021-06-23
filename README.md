go-wrk - an HTTP benchmarking tool
==================================

This is a fork of the wonderful https://github.com/tsliwowicz/go-wrk I did not create the code, only added a single new feature (better header parsing) https://github.com/tsliwowicz/go-wrk/pull/19 Use at your own risk.

Building
--------

    go get github.com/elodani/go-wrk

This will download and compile go-wrk. The binary will be placed under your $GOPATH/bin directory

Command line parameters (./go-wrk -help)

       Usage: go-wrk <options> <url>
       Options:
        -H 	 Request header (Default )
        -M 	 HTTP method (Default GET)
        -T 	 Socket/request timeout in ms (Default 1000)
        -body 	 request body string or @filename (Default )
        -c 	 Number of goroutines to use (concurrent connections) (Default 10)
        -ca 	 CA file to verify peer against (SSL/TLS) (Default )
        -cert 	 CA certificate file to verify peer against (SSL/TLS) (Default )
        -d 	 Duration of test in seconds (Default 10)
        -f 	 Playback file name (Default <empty>)
        -help 	 Print help (Default false)
        -host 	 Host Header (Default )
        -http 	 Use HTTP/2 (Default true)
        -key 	 Private key file name (SSL/TLS (Default )
        -no-c 	 Disable Compression - Prevents sending the "Accept-Encoding: gzip" header (Default false)
        -no-ka 	 Disable KeepAlive - prevents re-use of TCP connections between different HTTP requests (Default false)
        -redir 	 Allow Redirects (Default false)
        -v 	 Print version details (Default false)

Basic Usage
-----------

    ./go-wrk -c 80 -d 5  http://192.168.1.118:8080/json

This runs a benchmark for 5 seconds, using 80 go routines (connections)

Output:

    Running 10s test @ http://192.168.1.118:8080/json
      80 goroutine(s) running concurrently
       142470 requests in 4.949028953s, 19.57MB read
         Requests/sec:		28787.47
         Transfer/sec:		3.95MB
         Avg Req Time:		0.0347ms
         Fastest Request:	0.0340ms
         Slowest Request:	0.0421ms
         Number of Errors:	0


Benchmarking Tips
-----------------

  The machine running go-wrk must have a sufficient number of ephemeral ports
  available and closed sockets should be recycled quickly. To handle the
  initial connection burst the server's listen(2) backlog should be greater
  than the number of concurrent connections being tested.

Acknowledgements
----------------

  All credits due to https://github.com/tsliwowicz
