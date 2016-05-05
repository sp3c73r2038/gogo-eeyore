package eeyore

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const BAIDU_URL string = "http://r6.mo.baidu.com/v5/idfa/status"
const BAIDU_FROM string = "1013684a"
const BAIDU_TOKEN string = "FpOd3tLRjVaa1FBpZURPcklBQXZFNnpwM9ZrYXBoWk21"

func SendRequestBaiduIME(app App, ad Advertiser) []byte {

	idfa := strings.Join(app.IDFA, "\n")
	idfa += "\n"

	timestamp := int(time.Now().Unix())

	secret := get_secret_baidu_ime(timestamp, idfa)

	u := new(url.URL)
	q := u.Query()
	q.Set("from", BAIDU_FROM)
	q.Set("time", strconv.Itoa(timestamp))
	q.Set("secret", secret)
	url := fmt.Sprintf("%s?%s", BAIDU_URL, q.Encode())

	fmt.Println(url)
	fmt.Println(idfa)

	post_data := bytes.NewBuffer([]byte(idfa))
	req, err := http.NewRequest("POST", url, post_data)

	// 这里千万不能搞错，就是 text/plan ，因为很重要所以说三遍，没有看错
	// 这里千万不能搞错，就是 text/plan ，因为很重要所以说三遍，没有看错
	// 这里千万不能搞错，就是 text/plan ，因为很重要所以说三遍，没有看错
	req.Header.Set("Content-Type", "text/plan")

	if err != nil {
		panic(err)
	}

	timeout := time.Second * time.Duration(ad.Timeout)
	client := &http.Client{
		Timeout: timeout,
	}

	fmt.Println("sending request to", url)

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

func get_secret_baidu_ime(timestamp int, idfa string) string {
	m := md5.New()
	io.WriteString(m, BAIDU_FROM)
	io.WriteString(m, strconv.Itoa(timestamp))
	io.WriteString(m, BAIDU_TOKEN)
	io.WriteString(m, idfa)
	return fmt.Sprintf("%x", m.Sum(nil))
}
