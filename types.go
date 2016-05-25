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
	Debug      bool
	Database   map[string]Database
	Redis      map[string]Redis
	Advertiser map[string]Advertiser
	Kafka      KafkaConfig
	Amqp       map[string]Amqp
	Statsd     Statsd
}

type App struct {
	AdvertiserId int64          `codec:"ad_id"`
	Url          string         `codec:"url"`
	AppleId      int64          `codec:"apple_id"`
	IdfaType     int            `codec:"idfa_type"`
	IDFA         []string       `codec:"idfas,omitempty"`
	Result       map[string]int `codec:"result,omitempty"`
}
