package main

import (
	"crypto/sha1"
	"eeyore"
	"flag"
	"fmt"
	"io"
	"log"
	"runtime"
	"time"
)

import (
	"github.com/garyburd/redigo/redis"
	"github.com/streadway/amqp"
	"github.com/ugorji/go/codec"
)

type InputPayload struct {
	UserId int64  `codec:"uid"`
	Idfa   string `codec:"idfa"`
}

var (
	configFile = flag.String("config", "eeyore.toml", "config file")
	worker     = flag.Int("worker", 8, "worker number")
)

var config eeyore.Config
var r map[string]*redis.Pool
var todo_apps map[int64]eeyore.App

func logError(err error) {
	if err != nil {
		log.Println(err)
	}
}

func getAmqpConfig(bind string) eeyore.Amqp {
	return config.Amqp[bind]
}

func setCheckTiming(user_id int64, apple_id int64) {
	client := r["monitor"].Get()
	defer client.Close()

	key := fmt.Sprintf(
		"qianka:eeyore:check_timing_%d_%d", user_id, apple_id)
	value := int(time.Now().Unix())

	// 30 min
	_, err := client.Do("SETEX", key, 1800, value)
	logError(err)
}

func newRedisPool(server eeyore.Redis) *redis.Pool {
	return &redis.Pool{
		MaxActive:   1024,
		MaxIdle:     16,
		IdleTimeout: 60 * time.Second,
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

	r["result"] = newRedisPool(config.Redis["result"])
	r["backend"] = newRedisPool(config.Redis["backend"])
	r["monitor"] = newRedisPool(config.Redis["monitor"])
}

func get_todo_apps() {
	client := r["backend"].Get()
	defer client.Close()

	payload, err := client.Do("GET", "qianka:eeyore:todo_apps")
	logError(err)

	if payload == nil {
		log.Println("no todo_apps found")
		return
	}

	var h codec.MsgpackHandle

	var _apps map[int64]eeyore.App
	var dec *codec.Decoder = codec.NewDecoderBytes(payload.([]byte), &h)
	err = dec.Decode(&_apps)

	if err == nil {
		todo_apps = make(map[int64]eeyore.App)
		todo_apps = _apps
	}
}

func decodeInputPayload(payload []byte) InputPayload {
	var rv InputPayload
	var h codec.JsonHandle
	var dec *codec.Decoder = codec.NewDecoderBytes(payload, &h)
	err := dec.Decode(&rv)
	logError(err)
	return rv
}

func appendPayload(apple_id int64, idfa string) {
	key := fmt.Sprintf("qianka:eeyore:app_%d_idfas", apple_id)
	client := r["backend"].Get()
	defer client.Close()

	payload, err := eeyore.MsgpackPackb(idfa)
	logError(err)
	if err != nil {
		return
	}
	m := sha1.New()
	io.WriteString(m, idfa)
	hash := fmt.Sprintf("%x", m.Sum(nil))[:8]

	idx_key := fmt.Sprintf("qianka:eeyore:app_%d_idfas:idx", apple_id)
	exists, err := client.Do("SISMEMBER", idx_key, hash)

	if exists == 1 {
		return
	}

	_, err = client.Do("SADD", idx_key, hash)
	logError(err)
	_, err = client.Do("RPUSH", key, payload)
	logError(err)
}

func collectorWorker() {
	amqpConfig := getAmqpConfig("frontend")

	// log.Printf("%+v", amqpConfig)
	conn, err := amqp.Dial(amqpConfig.Url)
	defer conn.Close()

	logError(err)

	if err != nil {
		panic(err)
	}

	ch, err := conn.Channel()
	logError(err)
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"hera.idfa-check-frontend", // name
		false, // durable
		true,  // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	logError(err)

	err = ch.QueueBind(
		q.Name,
		amqpConfig.RoutingKey, // routing key
		amqpConfig.Exchange,   // exchange,
		false,                 // nowait
		nil,                   // arguments
	)
	logError(err)

	err = ch.Qos(0, 0, false)
	logError(err)

	msgs, err := ch.Consume(
		q.Name,
		"",    // consumer
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // arguments
	)
	logError(err)

	for d := range msgs {
		// log.Println(string(d.Body))
		d.Ack(false)
		processMessage(d.Body)
	}

}

func ackWorker(ack chan amqp.Delivery, pipe chan []byte) {
	for d := range ack {
		d.Ack(true)
		pipe <- d.Body
	}
}

func processWorker(pipe chan []byte) {
	for p := range pipe {
		processMessage(p)
	}
}

func processMessage(payload []byte) {
	inputPayload := decodeInputPayload(payload)

	// log.Printf("%+v", inputPayload)

	if len(todo_apps) == 0 {
		log.Println("no todo apps")
		return
	}

	client := r["backend"].Get()
	defer client.Close()
	key := fmt.Sprintf("qianka:eeyore:checked_user_%d", inputPayload.UserId)
	_p, err := client.Do("GET", key)
	logError(err)
	if _p != nil {
		// checked user
		return
	}

	// cache checked user
	_, err = client.Do("SETEX", key, 900, 1)
	logError(err)

	for apple_id := range todo_apps {
		appendPayload(apple_id, inputPayload.Idfa)
		setCheckTiming(inputPayload.UserId, apple_id)
	}

	// save idfa -> user mapping
	key = fmt.Sprintf("qianka:eeyore:idfa_%s_user", inputPayload.Idfa)
	_payload, err := eeyore.MsgpackPackb(inputPayload.UserId)
	logError(err)
	_, err = client.Do(
		"SETEX", key, 86400, _payload)
	logError(err)
}

func todoAppsCollector() {
	for {
		time.Sleep(time.Second * 30)
		get_todo_apps()
	}
}

func main() {
	flag.Parse()
	config = eeyore.LoadConfig(*configFile)
	log.Printf("%+v", config)

	initRedis()
	get_todo_apps()
	go todoAppsCollector()

	log.Println("GOMAXPROCS:", runtime.GOMAXPROCS(1024))

	for i := 0; i < *worker; i++ {
		go collectorWorker()
	}

	for {
		time.Sleep(time.Second * 10)
	}
}
