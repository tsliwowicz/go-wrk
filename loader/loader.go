package loader

import (
	"fmt"
	"github.com/tsliwowicz/go-wrk/util"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"
)

var interrupted int32 = 0

type LoadCfg struct {
	duration           int //seconds
	goroutines         int
	testUrl            string
	method             string
	statsAggregator    chan *RequesterStats
	timeoutms          int
	allowRedirects     bool
	disableCompression bool
	disableKeepAlive   bool
}

// RequesterStats used for colelcting aggregate statistics
type RequesterStats struct {
	TotRespSize    int64
	TotDuration    time.Duration
	MinRequestTime time.Duration
	MaxRequestTime time.Duration
	NumRequests    int
	NumErrs        int
}

func NewLoadCfg(duration int, //seconds
	goroutines int,
	testUrl string,
	method string,
	statsAggregator chan *RequesterStats,
	timeoutms int,
	allowRedirects bool,
	disableCompression bool,
	disableKeepAlive bool) (rt LoadCfg) {
	rt = LoadCfg{duration, goroutines, testUrl, method, statsAggregator, timeoutms,
		allowRedirects, disableCompression, disableKeepAlive}
	return
}

//DoRequest single request implementation. Returns the size of the response and its duration
//On error - returns -1 on both
func (cfg LoadCfg) DoRequest(httpClient *http.Client) (respSize int, duration time.Duration) {
	respSize = -1
	duration = -1
	req, err := http.NewRequest(cfg.method, cfg.testUrl, nil)

	req.Header.Add("User-Agent", "go-wrk")
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
			respSize = len(body) + int(util.EstimateHttpHeadersSize(resp.Header))
		}
	} else if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusTemporaryRedirect {
		duration = time.Since(start)
		respSize = int(resp.ContentLength) + int(util.EstimateHttpHeadersSize(resp.Header))
	}

	return
}

//Requester a go function for repeatedly making requests and aggregating statistics as long as required
//When it is done, it sends the results using the statsAggregator channel
func (cfg LoadCfg) Requester() {
	stats := &RequesterStats{MinRequestTime: time.Minute}
	start := time.Now()
	var httpClient *http.Client

	if cfg.allowRedirects {
		httpClient = &http.Client{}
	} else {
		//returning an error when trying to redirect. This prevents the redirection from happening.
		httpClient = &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return util.NewRedirectError("redirection not allowed")
		}}
	}

	//overriding the default parameters
	httpClient.Transport = &http.Transport{
		DisableCompression:    cfg.disableCompression,
		DisableKeepAlives:     cfg.disableKeepAlive,
		ResponseHeaderTimeout: time.Millisecond * time.Duration(cfg.timeoutms),
	}

	for time.Since(start).Seconds() <= float64(cfg.duration) && atomic.LoadInt32(&interrupted) == 0 {
		respSize, reqDur := cfg.DoRequest(httpClient)
		if respSize > 0 {
			stats.TotRespSize += int64(respSize)
			stats.TotDuration += reqDur
			stats.MaxRequestTime = util.MaxDuration(reqDur, stats.MaxRequestTime)
			stats.MinRequestTime = util.MinDuration(reqDur, stats.MinRequestTime)
			stats.NumRequests++
		} else {
			stats.NumErrs++
		}
	}
	cfg.statsAggregator <- stats
}

func Stop() {
	atomic.StoreInt32(&interrupted, 1)
}
