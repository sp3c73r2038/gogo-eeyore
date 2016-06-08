package main

import (
	"bytes"
	"crypto/md5"
	"eeyore"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"
)

import (
	"github.com/garyburd/redigo/redis"
	"github.com/ugorji/go/codec"
)

var (
	configFile = flag.String("config", "eeyore.toml", "config file")
	worker     = flag.Int("worker", 32, "worker number")
)

var config eeyore.Config
var r map[string]*redis.Pool

func newRedisPool(server eeyore.Redis) *redis.Pool {
	return &redis.Pool{
		MaxActive:   1024,
		MaxIdle:     16,
		IdleTimeout: 5 * time.Second,
		Dial: func() (redis.Conn, error) {
			address := fmt.Sprintf(
				"%s:%d", server.Host, server.Port)
			c, err := redis.Dial("tcp", address)
			if err != nil {
				return nil, err
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

func initRedis() {
	r = make(map[string]*redis.Pool)

	r["backend"] = newRedisPool(config.Redis["backend"])
}

func process_message(app eeyore.App) {
	// fmt.Printf("app: %+v\n", app)

	var ad eeyore.Advertiser

	ad_id := strconv.FormatInt(app.AdvertiserId, 10)
	if val, ok := config.Advertiser[ad_id]; ok {
		ad = val
		ad.Id = app.AdvertiserId

		if ad.Impl == "" {
			ad.Impl = "base"
		}
		if ad.Timeout == 0 {
			ad.Timeout = 30
		}

	} else {
		ad = eeyore.Advertiser{
			Id:        app.AdvertiserId,
			Impl:      "base",
			Timeout:   30,
			SharedKey: "",
		}
	}

	// fmt.Printf("%+v\n", ad)

	var text []byte
	if ad.Impl == "base" {
		text = send_request(app, ad)
	} else if ad.Impl == "jd" {
		text = eeyore.SendRequestJD(app, ad)
	} else if ad.Impl == "baidu_ime" {
		text = eeyore.SendRequestBaiduIME(app, ad)
	} else if ad.Impl == "qijia" {
		text = eeyore.SendRequestQijia(app, ad)
	} else if ad.Impl == "zhangyue" {
		text = eeyore.SendRequestZhangyue(app, ad)
	} else {
		log.Println("unknown impl:", ad.Impl)
		return
	}

	// fmt.Println("text:", string(text))

	if len(text) <= 0 {
		log.Println("no text")
		return
	}

	var mapping map[string]int
	if ad.Impl == "jd" {
		mapping = eeyore.HandleResponseJD(text)
	} else if ad.Impl == "zhangyue" {
		mapping = eeyore.HandleResponseZhangyue(text)
	} else {
		mapping = handle_response(text)
	}

	if len(mapping) == 0 {
		log.Println("not valid response")
		return
	}

	app.Result = mapping

	push(app)
}

// pop from pending_send queue
// * including msgpack unpack
// * will return empty App if no response
func pop() eeyore.App {
	// fmt.Println("poping...")

	var app eeyore.App

	client := r["backend"].Get()
	defer client.Close()

	info, err := client.Do("BLPOP",
		"qianka:eeyore:pending_send", 0)

	if err != nil {
		r = nil
		log.Printf("pop: %T\n", err)
		log.Printf("pop: %+v\n", err)
		initRedis()
		return app
	}

	// fmt.Println(info[1])

	var h codec.MsgpackHandle
	h.RawToString = true

	payload := info.([]interface{})[1].([]byte)

	var dec *codec.Decoder = codec.NewDecoderBytes(payload, &h)
	err = dec.Decode(&app)

	// log.Printf("%+v", app)

	if err != nil {
		panic(err)
	}

	return app
}

// push to pending_cache queue
func push(app eeyore.App) {
	var b []byte
	var h codec.Handle = new(codec.MsgpackHandle)
	var enc *codec.Encoder = codec.NewEncoderBytes(&b, h)
	err := enc.Encode(app)

	if err != nil {
		panic(err)
	}

	client := r["backend"].Get()
	info, err := client.Do("RPUSH", "qianka:eeyore:pending_cache", b)

	if err != nil {
		r = nil
		log.Println("push:", err)
		initRedis()
	}

	if info == 1 {
	}

}

func get_sign(
	apple_id int64, idfa string, timestamp int, shared_key string) string {
	m := md5.New()

	io.WriteString(m, strconv.FormatInt(apple_id, 10))
	io.WriteString(m, idfa)
	io.WriteString(m, strconv.Itoa(timestamp))
	io.WriteString(m, shared_key)
	return fmt.Sprintf("%x", m.Sum(nil))
}

// send_request `the standard` version
// * send request and fetch body
func send_request(app eeyore.App, ad eeyore.Advertiser) []byte {

	u := new(url.URL)
	q := u.Query()

	q.Set("appid", strconv.FormatInt(app.AppleId, 10))
	idfa := strings.Join(app.IDFA, ",")
	q.Set("idfa", idfa)

	if ad.SharedKey != "" {
		timestamp := int(time.Now().Unix())
		q.Set("timestamp", strconv.Itoa(timestamp))
		sign := get_sign(app.AppleId, idfa, timestamp, ad.SharedKey)
		q.Set("sign", sign)

		if ad.CallerId != "" {
			q.Set("callerid", ad.CallerId)
		}

	}

	body := q.Encode()
	// fmt.Println(body)
	post_data := bytes.NewBuffer([]byte(body))
	req, err := http.NewRequest("POST", app.Url, post_data)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if err != nil {
		panic(err)
	}

	timeout := time.Second * time.Duration(ad.Timeout)
	client := &http.Client{
		Timeout: timeout,
	}

	log.Println("sending request to", app.Url)

	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return []byte("")
	}

	// fmt.Println("status:", resp.StatusCode)

	defer resp.Body.Close()

	text, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		log.Println("status not 200:", resp.StatusCode)
		log.Println("response:", string(text))
		return []byte("")
	}

	return text
}

// handle response `the standard` version
// * JSON decode
func handle_response(text []byte) map[string]int {
	var m map[string]int
	err := codec.NewDecoderBytes(text, new(codec.JsonHandle)).Decode(&m)

	if err != nil {
		log.Println("decode error", err)
		log.Println(string(text))
		return m
	}

	return m

}

func main() {

	flag.Parse()
	config = eeyore.LoadConfig(*configFile)
	log.Printf("eeyore Config: %+v\n", config)
	log.Println("available GOMAXPROCS:", runtime.GOMAXPROCS(*worker))

	initRedis()

	var app eeyore.App

	for {
		app = pop()
		// fmt.Println("apple_id:", app.AppleId)
		if app.AppleId == 0 {
			continue
		}
		go process_message(app)
	}
}
