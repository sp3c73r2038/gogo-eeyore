package main

import (
	"crypto/md5"
	"eeyore"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

import (
	"github.com/ugorji/go/codec"
)

var (
	configFile = flag.String("config", "eeyore.toml", "config file")
	worker     = flag.Int("worker", 32, "worker number")
)

var config eeyore.Config
var r map[string]*eeyore.RedisPool
var loader eeyore.IDFAConfigLoader

// var loader eeyore.IDFAConfigLoader

func process_message(q chan eeyore.App) {
	// fmt.Printf("app: %+v\n", app)
	var app eeyore.App
	var ad eeyore.Advertiser
	var idfa_config eeyore.IdfaConfig

	for {
		app = <-q

		idfa_config = loader.GetByApp(app.AppleId)
		if idfa_config.AppId == 0 {
			idfa_config = loader.GetByAdvertiser(app.AdvertiserId)
		}

		if len(idfa_config.TemplateCode) > 0 {
			ad.Impl = idfa_config.TemplateCode
		}

		log.Printf("process_message(): %+v", idfa_config)
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

		var text []byte
		if ad.Impl == "base" {
			text = SendRequest(app, ad, idfa_config)
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
			continue
		}

		// fmt.Println("text:", string(text))

		if len(text) <= 0 {
			log.Println("no text")
			continue
		}

		var mapping map[string]int
		if ad.Impl == "jd" {
			mapping = eeyore.HandleResponseJD(text)
		} else if ad.Impl == "zhangyue" {
			mapping = eeyore.HandleResponseZhangyue(text)
		} else {
			mapping = HandleResponse(text, idfa_config)
		}

		if len(mapping) == 0 {
			log.Println("not valid response")
			continue
		}

		app.Result = mapping

		push(app)
	}

}

// pop from pending_send queue
// * including msgpack unpack
// * will return empty App if no response
func pop(q chan eeyore.App) {
	// fmt.Println("poping...")

	var app eeyore.App
	client := r["backend"].Get()
	defer client.Close()

	for {
		info, err := client.Do("BLPOP",
			"qianka:eeyore:pending_send", 0)

		if err != nil {
			r = nil
			log.Printf("pop(): %T\n", err)
			log.Printf("pop(): %+v\n", err)
			r = eeyore.InitRedis(
				*worker, []string{"backend"}, config)
			time.Sleep(time.Second * 1)
			continue
		}

		// fmt.Println(info[1])

		var h codec.MsgpackHandle
		h.RawToString = true

		payload := info.([]interface{})[1].([]byte)

		var dec *codec.Decoder = codec.NewDecoderBytes(payload, &h)
		err = dec.Decode(&app)

		// log.Printf("%+v", app)

		if err != nil {
			log.Println("pop():", err)
			continue
		}

		q <- app
	}
}

// push to pending_cache queue
func push(app eeyore.App) {
	var b []byte
	var h codec.Handle = new(codec.MsgpackHandle)
	var enc *codec.Encoder = codec.NewEncoderBytes(&b, h)
	err := enc.Encode(app)

	if err != nil {
		log.Println("push():", err)
		return
	}

	client := r["backend"].Get()
	info, err := client.Do("RPUSH", "qianka:eeyore:pending_cache", b)

	if err != nil {
		r = nil
		log.Println("push():", err)
		r = eeyore.InitRedis(
			*worker, []string{"backend"}, config)
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
func SendRequest(
	app eeyore.App, ad eeyore.Advertiser, idfa_config eeyore.IdfaConfig) []byte {

	var idfa string
	var data url.Values
	var sign string
	var timeout time.Duration
	var httpCode int
	var err error
	var rv []byte

	if len(idfa_config.SharedKey) > 0 {
		ad.SharedKey = idfa_config.SharedKey
	}

	data = url.Values{}
	data.Set("appid", strconv.FormatInt(app.AppleId, 10))

	idfa = strings.Join(app.IDFA, ",")
	if idfa_config.RequestIDFALowerCase {
		idfa = strings.ToLower(idfa)
	}

	if idfa_config.RequestIDFANoHyphen {
		idfa = strings.Replace(idfa, "-", "", -1)
	}

	data.Set("idfa", idfa)

	if ad.SharedKey != "" {
		timestamp := int(time.Now().Unix())
		data.Set("timestamp", strconv.Itoa(timestamp))
		sign = get_sign(app.AppleId, idfa, timestamp, ad.SharedKey)
		data.Set("sign", sign)

		if ad.CallerId != "" {
			data.Set("callerid", ad.CallerId)
		}
	}

	if idfa_config.AppId == 0 && idfa_config.AdvertiserId == 0 {
		timeout = time.Second * time.Duration(ad.Timeout)
		httpCode, rv, err = eeyore.HttpRequestPost(
			app.Url,
			timeout,
			data,
			"application/x-www-form-urlencoded",
		)

		if httpCode != 200 {
			log.Println("SendRequest() http not 200:", err)
		}
		return rv
	}

	/// use idfa_config to customize request
	// fill in default value
	idfa_config.RequestHttpMethod = strings.ToUpper(idfa_config.RequestHttpMethod)
	if len(idfa_config.RequestHttpMethod) == 0 {
		idfa_config.RequestHttpMethod = "POST"
	}

	httpCode, rv, err = eeyore.HttpRequest(
		idfa_config.RequestHttpMethod,
		app.Url,
		data,
		time.Second*time.Duration(ad.Timeout),
		idfa_config.RequestContentType,
	)

	if httpCode != 200 {
		log.Println("SendRequest() http not 200:", err)
	}
	return rv

}

// handle response `the standard` version
// * JSON decode
func HandleResponse(text []byte, idfa_config eeyore.IdfaConfig) map[string]int {
	var rv map[string]int
	var m map[string]int
	err := codec.NewDecoderBytes(text, new(codec.JsonHandle)).Decode(&m)

	if err != nil {
		log.Println("decode error", err)
		log.Println(string(text))
		return m
	}

	rv = make(map[string]int)
	for k, v := range m {
		rv[strings.ToUpper(k)] = v
	}

	if idfa_config.ResponseIDFANoHyphen {
		rv = IdfaFillHyphen(rv)
	}

	log.Printf("HandleResponse(): %+v", rv)

	return rv
}

func IdfaFillHyphen(data map[string]int) map[string]int {
	var rv map[string]int
	var newkey string
	var m bool

	rv = make(map[string]int)
	for k, v := range data {
		m, _ = regexp.MatchString(`^[A-Z0-9]{32}$`, k)
		if m {
			newkey = fmt.Sprintf(
				"%s-%s-%s-%s-%s", k[:8], k[8:12], k[12:16], k[16:20], k[20:])
			rv[newkey] = v
		}
	}

	return rv
}

func main() {

	flag.Parse()
	config = eeyore.LoadConfig(*configFile)
	log.Printf("eeyore Config: %+v\n", config)
	log.Println("available GOMAXPROCS:", runtime.GOMAXPROCS(*worker))

	r = eeyore.InitRedis((*worker * 5), []string{"backend"}, config)
	loader = eeyore.IDFAConfigLoader{
		BaseURL:     config.IDFAConfigBaseURL,
		CacheExpiry: 60,
	}
	loader.Init()

	q := make(chan eeyore.App)
	for i := 0; i < *worker; i++ {
		go pop(q)
	}
	for i := 0; i < *worker; i++ {
		go process_message(q)
	}

	for {
		time.Sleep(time.Second * 10)
	}
}
