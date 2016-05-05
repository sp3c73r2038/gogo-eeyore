package eeyore

import (
	"bytes"
	"fmt"
	"github.com/ugorji/go/codec"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Payload struct {
	AppleId int    `codec:"appid"`
	Source  string `codec:"source"`
	IDFA    string `codec:"idfa"`
}

func SendRequestQijia(app App, ad Advertiser) []byte {

	payload := Payload{}
	payload.AppleId, _ = strconv.Atoi(app.AppleId)
	payload.Source = "qianka"
	payload.IDFA = strings.Join(app.IDFA, ",")

	var b []byte
	var h codec.Handle = new(codec.JsonHandle)
	var enc *codec.Encoder = codec.NewEncoderBytes(&b, h)
	err := enc.Encode(payload)

	post_data := bytes.NewBuffer(b)
	req, err := http.NewRequest("POST", app.Url, post_data)
	req.Header.Set("Content-Type", "application/json")

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

	defer resp.Body.Close()

	text, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		fmt.Println("status not 200:", resp.StatusCode)
		fmt.Println("response:", string(text))
		return []byte("")
	}

	// fmt.Println("text:", string(text))

	return text
}
