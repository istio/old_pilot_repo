package main

import(
	"encoding/json"
	"fmt"
	"log"
	"time"
	"net/http"
	"io/ioutil"
)

func getReviews(forwardHeaders http.Header) map[string]*review {
	const attempts = 2
	const timeout = 3 * time.Second

	bytes, err := doRequest("http://reviews:9080/reviews", forwardHeaders, timeout, attempts)
	if err != nil {
		log.Printf("Error getting reviews: %v", err)
		return nil
	}

	reviews := map[string]*review{}
	err = json.Unmarshal(bytes, &reviews)
	if err != nil {
		log.Printf("Error unmarshaling reviews: %v", err)
		return nil
	}

	return reviews
}

func doRequest(path string, forwardHeaders http.Header, timeout time.Duration, attempts int) ([]byte, error) {
        client := http.Client{}
        client.Timeout = timeout

        for i := 0; i < attempts; i++ {
                req, _ := http.NewRequest("GET", path, nil)
                req.Header = forwardHeaders

                resp, err := client.Do(req)
                if err != nil {
                        log.Printf("Error executing HTTP request (%s %s): %v", req.Method, req.URL, err)
                        continue
                }
                if resp.StatusCode != http.StatusOK {
                        log.Printf("Error executing HTTP request (%s %s): received status code %d", req.Method, req.URL, resp.StatusCode)
                        continue
                }

                bytes, err := ioutil.ReadAll(resp.Body)
                if err != nil {
                        log.Printf("Error reading HTTP response (%s %s): %v", req.Method, req.URL, err)
                        continue
                }

                return bytes, nil
        }

        err := fmt.Errorf("run out of attempts")
        log.Printf("Error executing HTTP request (%s %s): %v", "GET", path, err)
        return nil, err
}

func getForwardHeaders(r *http.Request) http.Header {
        fwdReq, _ := http.NewRequest("GET", "dummy", nil)

        cookie, err := r.Cookie(userCookie)
        if err != http.ErrNoCookie {
                fwdReq.AddCookie(cookie)
        }

        reqID := r.Header.Get(requestIDHeader)
        if reqID != "" {
                fwdReq.Header.Set(requestIDHeader, reqID)
        }

        return fwdReq.Header
}
