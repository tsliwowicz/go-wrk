package loader

import (
	"fmt"
	"github.com/tsliwowicz/go-wrk/util"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const (
	USER_AGENT = "go-wrk"
)

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
	interrupted        int32
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
	disableKeepAlive bool) (rt *LoadCfg) {
	rt = &LoadCfg{duration, goroutines, testUrl, method, statsAggregator, timeoutms,
		allowRedirects, disableCompression, disableKeepAlive, 0}
	return
}

func escapeUrlStr(in string) string {
	qm := strings.Index(in, "?")
	if qm != -1 {
		qry := in[qm+1:]
		qrys := strings.Split(qry, "&")
		var query string = ""
		var qEscaped string = ""
		var first bool = true
		for _, q := range qrys {
			qSplit := strings.Split(q, "=")
			if len(qSplit) == 2 {
				qEscaped = qSplit[0] + "=" + url.QueryEscape(qSplit[1])
			} else {
				qEscaped = qSplit[0]
			}
			if first {
				first = false
			} else {
				query += "&"
			}
			query += qEscaped

		}
		return in[:qm] + "?" + query
	} else {
		return in
	}
}

//DoRequest single request implementation. Returns the size of the response and its duration
//On error - returns -1 on both
func DoRequest(httpClient *http.Client, method string, loadUrl string) (respSize int, duration time.Duration) {
	respSize = -1
	duration = -1

	loadUrl = escapeUrlStr(loadUrl)

	req, err := http.NewRequest(method, loadUrl, nil)
	if err != nil {
		fmt.Println("An error occured doing request", err)
		return
	}

	req.Header.Add("User-Agent", USER_AGENT)
	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Println("redirect?")
		//this is a bit weird. When redirection is prevented, a url.Error is retuned. This creates an issue to distinguish
		//between an invalid URL that was provided and and redirection error.
		rr, ok := err.(*url.Error)
		if !ok {
			fmt.Println("An error occured doing request", err, rr)
			return
		}
	}
	if resp == nil {
		fmt.Println("empty response")
		return
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("An error occured reading body", err)
	}
	if resp.StatusCode == http.StatusOK {
		duration = time.Since(start)
		respSize = len(body) + int(util.EstimateHttpHeadersSize(resp.Header))
	} else if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusTemporaryRedirect {
		duration = time.Since(start)
		respSize = int(resp.ContentLength) + int(util.EstimateHttpHeadersSize(resp.Header))
	} else {
		fmt.Println("received status code", resp.StatusCode, "from", resp.Header, "content", string(body), req)
	}

	return
}

//Requester a go function for repeatedly making requests and aggregating statistics as long as required
//When it is done, it sends the results using the statsAggregator channel
func (cfg *LoadCfg) RunSingleLoadSession() {
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

	for time.Since(start).Seconds() <= float64(cfg.duration) && atomic.LoadInt32(&cfg.interrupted) == 0 {
		respSize, reqDur := DoRequest(httpClient, cfg.method, cfg.testUrl)
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

func (cfg *LoadCfg) Stop() {
	atomic.StoreInt32(&cfg.interrupted, 1)
}
