package eeyore

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ugorji/go/codec"
)

const JD_UNIONID string = "350268123"
const JD_SECRET string = "NBQK-7J89-JKDS-RMNW-DEBC-2H8D-KH"
const JD_URL string = "http://adcollect.m.jd.com/queryAtivationByIdfa.do"

type Item struct {
	IDFA       string `codec:"idfa"`
	ClientType int    `codec:"clientType"`
	Error      string `codec:"error"`

	// 京东文档与接口中返回数据的key真的就是 `ativation`
	Activation bool `codec:"ativation"`
}

type Response struct {
	Result []Item `codec:"result"`
}

func SendRequestJD(app App, ad Advertiser) []byte {

	client_type := 0
	if app.AppleId == 414245413 {
		// 商城
		client_type = 1
	} else if app.AppleId == 895682747 {
		// 金融
		client_type = 4
	} else if app.AppleId == 832444218 {
		// 钱包
		client_type = 7
	} else if app.AppleId == 506583396 {
		// 阅读
		client_type = 10
	} else {
		log.Println("jd: unknown app id", app.AppleId)
		return []byte("")
	}

	var items []Item
	for i := range app.IDFA {
		items = append(items, Item{
			IDFA:       app.IDFA[i],
			ClientType: client_type,
		})
	}

	var b []byte
	var h codec.Handle = new(codec.JsonHandle)
	var enc *codec.Encoder = codec.NewEncoderBytes(&b, h)
	err := enc.Encode(items)
	body := string(b)

	m := md5.New()
	io.WriteString(m, body)
	io.WriteString(m, JD_SECRET)
	sign := fmt.Sprintf("%x", m.Sum(nil))

	u := new(url.URL)
	q := u.Query()
	q.Set("unionId", JD_UNIONID)
	q.Set("body", body)
	q.Set("sign", strings.ToUpper(sign))

	payload := q.Encode()

	// fmt.Println(payload)
	post_data := bytes.NewBuffer([]byte(payload))
	req, err := http.NewRequest("POST", JD_URL, post_data)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if err != nil {
		panic(err)
	}

	timeout := time.Second * time.Duration(ad.Timeout)
	client := &http.Client{
		Timeout: timeout,
	}

	log.Println("sending request to", JD_URL)

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

func HandleResponseJD(text []byte) map[string]int {
	rv := make(map[string]int)
	var m Response

	// fmt.Println(string(text))

	err := codec.NewDecoderBytes(text, new(codec.JsonHandle)).Decode(&m)

	if err != nil {
		panic(err)
	}

	for _, v := range m.Result {
		idfa := v.IDFA
		status := 0
		if v.Activation {
			status = 1
		}
		rv[idfa] = status
	}

	// fmt.Printf("%+v\n", rv)
	return rv
}
