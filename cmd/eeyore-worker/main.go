package main

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"eeyore"

	"github.com/ugorji/go/codec"
	"gopkg.in/redis.v3"
)

var config eeyore.Config
var r *redis.Client

func redis_connect() {
	if r == nil {
		r = redis.NewClient(&redis.Options{
			Addr: fmt.Sprintf(
				"%s:%d",
				config.Redis["backend"].Host,
				config.Redis["backend"].Port),
			Password: config.Redis["backend"].Pass,
			DB:       config.Redis["backend"].Db,
		})
	}
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
	} else {
		fmt.Println("unknown impl:", ad.Impl)
		return
	}

	// fmt.Println("text:", string(text))

	if len(text) <= 0 {
		fmt.Println("no text")
		return
	}

	var mapping map[string]int
	if ad.Impl == "jd" {
		mapping = eeyore.HandleResponseJD(text)
	} else {
		mapping = handle_response(text)
	}

	if len(mapping) == 0 {
		fmt.Println("not valid response")
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
	redis_connect()

	var app eeyore.App

	info, err := r.BLPop(
		time.Second*5, "qianka:eeyore:pending_send").Result()

	if err == redis.Nil {
		fmt.Println("no pending payload")
		return app
	} else if err != nil {
		r = nil
		fmt.Printf("%T\n", err)
		fmt.Printf("%+v\n", err)
		return app
	}

	// fmt.Println(info[1])

	var h codec.MsgpackHandle
	h.RawToString = true

	var dec *codec.Decoder = codec.NewDecoderBytes([]byte(info[1]), &h)
	err = dec.Decode(&app)

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

	redis_connect()

	info, err := r.RPush(
		"qianka:eeyore:pending_cache", string(b)).Result()

	if err != nil {
		r = nil
		fmt.Println(err)
	}

	if info == 1 {
	}

}

func get_sign(
	apple_id string, idfa string, timestamp int, shared_key string) string {
	m := md5.New()

	io.WriteString(m, apple_id)
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

	q.Set("appid", app.AppleId)
	idfa := strings.Join(app.IDFA, ",")
	q.Set("idfa", idfa)

	if ad.SharedKey != "" {
		timestamp := int(time.Now().Unix())
		q.Set("timestamp", strconv.Itoa(timestamp))
		sign := get_sign(app.AppleId, idfa, timestamp, ad.SharedKey)
		q.Set("sign", sign)
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

	fmt.Println("sending request to", app.Url)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return []byte("")
	}

	// fmt.Println("status:", resp.StatusCode)

	defer resp.Body.Close()

	text, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		fmt.Println("status not 200:", resp.StatusCode)
		fmt.Println("response:", string(text))
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
		fmt.Println("decode error", err)
		fmt.Println(string(text))
		return m
	}

	return m

}

func main() {

	// prog := os.Args[0]

	config_filename := ""

	if len(os.Args) == 1 {
		config_filename = "eeyore.toml"
	} else {
		config_filename = os.Args[1]
	}

	config = eeyore.LoadConfig(config_filename)
	fmt.Printf("eeyore Config: %+v\n", config)

	var app eeyore.App

	for {
		app = pop()
		// fmt.Println("apple_id:", app.AppleId)
		if app.AppleId == "" {
			continue
		}
		go process_message(app)
	}
}
