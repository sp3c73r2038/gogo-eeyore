package eeyore

import (
	"log"
	"net/url"
	"strconv"
	"time"
)

import (
	"github.com/ugorji/go/codec"
)

type IDFAConfigLoader struct {
	BaseURL     string
	CacheExpiry int

	configByAdvertiserId map[int64]IdfaConfig
	configByAppId        map[int64]IdfaConfig
}

func (i *IDFAConfigLoader) Init() {
	i.configByAppId = make(map[int64]IdfaConfig)
	i.configByAdvertiserId = make(map[int64]IdfaConfig)
}

func (i *IDFAConfigLoader) GetByAdvertiser(
	advertiser_id int64) IdfaConfig {

	var ok bool
	var config IdfaConfig

	// get from in-memory cache
	if config, ok = i.configByAdvertiserId[advertiser_id]; ok {
		if int(time.Now().Unix())-config.Timestamp < i.CacheExpiry {
			return config
		}
	}

	var _url string
	var timeout time.Duration
	var text []byte
	var err error
	var rv IdfaConfig
	var httpCode int
	var data url.Values

	_url = i.BaseURL
	data = url.Values{}
	data.Set("ad_id", strconv.FormatInt(advertiser_id, 10))

	httpCode, text, _ = HttpRequest(
		"GET",   // method
		_url,    // url
		data,    // data
		timeout, // timeout
		"",      // content-type
	)

	if httpCode != 200 {
		return rv
	}

	var c *IdfaConfigResponse = &IdfaConfigResponse{}
	var h codec.JsonHandle
	var dec *codec.Decoder = codec.NewDecoderBytes(text, &h)
	err = dec.Decode(c)

	if err != nil {
		log.Println("HttpRequestGet():", err)
		return rv
	}

	log.Printf("GetByAdvertiser(): %+v", c)

	// log.Printf("%+v", c)
	if len(c.Data) > 0 {
		rv = c.Data[0]
		if rv.Enabled {
			rv.Timestamp = int(time.Now().Unix())
			// set in-memory cache
			i.configByAdvertiserId[advertiser_id] = rv
		}
	}
	return rv
}

func (i *IDFAConfigLoader) GetByApp(
	app_id int64) IdfaConfig {

	var ok bool
	var config IdfaConfig

	// get from in-memory cache
	if config, ok = i.configByAppId[app_id]; ok {
		if int(time.Now().Unix())-config.Timestamp < i.CacheExpiry {
			return config
		}
	}

	var _url string
	var timeout time.Duration
	var text []byte
	var err error
	var rv IdfaConfig
	var httpCode int
	var data url.Values

	_url = i.BaseURL
	data = url.Values{}
	data.Set("app_id", strconv.FormatInt(app_id, 10))

	httpCode, text, _ = HttpRequest(
		"GET",   // method
		_url,    // url
		data,    // data
		timeout, // timeout
		"",      // content-type
	)

	if httpCode != 200 {
		return rv
	}

	var c *IdfaConfigResponse = &IdfaConfigResponse{}
	var h codec.JsonHandle
	var dec *codec.Decoder = codec.NewDecoderBytes(text, &h)
	err = dec.Decode(c)

	if err != nil {
		log.Println("HttpRequestGet():", err)
		return rv
	}

	log.Printf("%+v", c)

	// log.Printf("%+v", c)
	if len(c.Data) > 0 {
		rv = c.Data[0]
		if rv.Enabled {
			rv.Timestamp = int(time.Now().Unix())
			// set in-memory cache
			i.configByAppId[app_id] = rv
		}
	}
	return rv
}
