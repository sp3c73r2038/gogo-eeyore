package eeyore

type Database struct {
	Host string
	Port int
	User string
	Pass string
	Db   string
}

type Redis struct {
	Host string
	Port int
	Pass string
	Db   int64
}

type Amqp struct {
	Url        string
	Exchange   string
	RoutingKey string
}

type Statsd struct {
	Endpoint string
}

type Advertiser struct {
	Id        int64
	Impl      string
	Timeout   int
	SharedKey string
	CallerId  string
}

type Config struct {
	Debug             bool
	Database          map[string]Database
	Redis             map[string]Redis
	Advertiser        map[string]Advertiser
	Kafka             KafkaConfig
	Amqp              map[string]Amqp
	Statsd            Statsd
	IDFAConfigBaseURL string
}

type App struct {
	AdvertiserId int64          `codec:"ad_id"`
	Url          string         `codec:"url"`
	AppleId      int64          `codec:"apple_id"`
	IdfaType     int            `codec:"idfa_type"`
	IDFA         []string       `codec:"idfas,omitempty"`
	Result       map[string]int `codec:"result,omitempty"`
}

type IdfaConfig struct {
	AdvertiserId         int64  `codec:"ad_id"`
	AppId                int64  `codec:"app_id"`
	Enabled              bool   `codec:"is_enabled"`
	TemplateCode         string `codec:"template_code"`
	SharedKey            string `codec:"shared_key"`
	RequestContentType   string `codec:"req_content_type"`
	RequestHttpMethod    string `codec:"req_http_method"`
	RequestIDFALowerCase bool   `codec:"req_idfa_lowercase"`
	RequestIDFAMaxNum    int    `codec:"req_idfa_maxnum"`
	RequestIDFANoHyphen  bool   `codec:"req_idfa_nohyphen"`
	ResponseIDFANoHyphen bool   `codec:"res_idfa_nohyphen"`
	ResponseJSONFormat   string `codec:"res_json_format"`
	ExtendedField        string `codec:"ext_field"`
	Timestamp            int
}

type IdfaConfigResponse struct {
	Status string       `codec:"status"`
	Data   []IdfaConfig `codec:"data"`
}
