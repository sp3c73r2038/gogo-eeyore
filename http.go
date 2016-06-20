package eeyore

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"
)

func HttpRequest(
	method string, _url string,
	data url.Values, timeout time.Duration,
	content_type string) (int, []byte, error) {

	var u *url.URL
	var q url.Values
	var err error

	if method == "GET" {
		u, err = url.Parse(_url)

		if err != nil {
			log.Println("HttpRequest():", err)
			return 0, []byte(""), err
		}

		if data != nil {
			q = u.Query()
			for k, v := range data {
				for _v := range v {
					q.Add(k, v[_v])
				}
			}
			u.RawQuery = q.Encode()

		}
		log.Printf("HttpRequest() url: %+v", u.String())
		return HttpRequestGet(u.String(), timeout, content_type)
	} else {
		return HttpRequestNonGet(method, _url, data, timeout, content_type)
	}
}

func HttpRequestGet(
	url string, timeout time.Duration, content_type string) (int, []byte, error) {
	var req *http.Request
	var resp *http.Response
	var client *http.Client
	var text []byte
	var err error

	req, err = http.NewRequest("GET", url, nil)
	if len(content_type) > 0 {
		req.Header.Set("Content-Type", content_type)
	}

	client = &http.Client{
		Timeout: timeout,
	}

	resp, err = client.Do(req)
	if err != nil {
		log.Println("HttpRequestGet():", err)
		return 0, []byte(""), err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Println("HttpRequestGet() status not 200:", err)
		return resp.StatusCode, []byte(""), err
	}

	text, _ = ioutil.ReadAll(resp.Body)
	// log.Printf("HttpRequestGet() text: %+v", string(text))
	return resp.StatusCode, text, err

}

func HttpRequestPost(
	_url string, timeout time.Duration,
	data url.Values, content_type string) (int, []byte, error) {
	return HttpRequestNonGet("POST", _url, data, timeout, content_type)
}

func HttpRequestNonGet(
	method string, _url string,
	data url.Values, timeout time.Duration,
	content_type string) (int, []byte, error) {

	var body *bytes.Buffer
	var req *http.Request
	var resp *http.Response
	var client *http.Client
	var err error

	if method != "GET" && method != "POST" {
		log.Printf("HttpRequestNonGet(): unsupported method %s", method)
		return 0, []byte(""), err
	}

	body = bytes.NewBuffer([]byte(data.Encode()))
	req, err = http.NewRequest(method, _url, body)

	if err != nil {
		log.Println("HttpRequestNonGet():", err)
		return 0, []byte(""), err
	}

	if len(content_type) > 0 {
		req.Header.Set("Content-Type", content_type)
	} else {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	client = &http.Client{
		Timeout: timeout,
	}

	log.Printf("sending %s request to %s", method, _url)

	resp, err = client.Do(req)
	if err != nil {
		log.Println("HttpRequestNonGet():", err)
		return 0, []byte(""), err
	}

	defer resp.Body.Close()
	text, _ := ioutil.ReadAll(resp.Body)

	return resp.StatusCode, text, err
}
