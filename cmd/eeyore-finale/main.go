package main

import (
	"eeyore"
	"flag"
	"fmt"
	"log"
	"time"
)

import (
	"github.com/garyburd/redigo/redis"
	"github.com/ugorji/go/codec"
)

var (
	config_file = flag.String("config", "eeyore.toml", "config file")
	worker      = flag.Int("worker", 32, "worker number")
)

var config eeyore.Config
var r map[string]*redis.Pool
var cnt int64 = 0
var logger eeyore.KafkaLogger

func initKafkaLogger() {
	logger = eeyore.KafkaLogger{}
	logger.Init(config.Kafka)
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
}

func write_check_result(user_id int64, apple_id int64, status int) error {
	key := fmt.Sprintf("hera:idfa-check:result_%d_%d", user_id, apple_id)
	value := fmt.Sprintf("%d %d", status, int(time.Now().Unix()))

	client := r["result"].Get()
	defer client.Close()

	_, err := client.Do("SET", key, value)
	return err
}

// pop from pending_cache queue
// * include msgpack unpack
func pop(q chan []byte) {
	client := r["backend"].Get()
	defer client.Close()

	for {
		info, err := client.Do("BLPOP",
			"qianka:eeyore:pending_cache", 5)

		if err != nil {
			// r["backend"] = nil
			log.Printf("pop error: %+v", err)
			continue
		} else if info == nil {
			log.Println("no pending payload")
			continue
		}

		payload := info.([]interface{})[1].([]byte)
		q <- payload
		cnt++
	}
}

// push to pending_cache queue
func push(b []byte) {
	var err error

	client := r["backend"].Get()
	defer client.Close()

	_, err = client.Do(
		"RPUSH", "qianka:eeyore:pending_write_back", b)

	if err != nil {
		// r["backend"] = nil
		log.Println("push error", err)
	}
}

//
func get_user_id_by_idfa(idfa string) (int64, error) {
	var rv int64
	var key string
	var err error
	key = fmt.Sprintf("qianka:eeyore:idfa_%s_user", idfa)

	client := r["backend"].Get()
	defer client.Close()

	info, err := client.Do("GET", key)

	if info == nil {
		return rv, err
	}

	payload := info.([]byte)

	if err != nil {
		log.Println("get_user_id_by_idfa redis:", err)
		return rv, err
	}

	var h codec.MsgpackHandle

	var dec *codec.Decoder = codec.NewDecoderBytes(payload, &h)
	err = dec.Decode(&rv)
	log.Println("user_id:", rv)

	if err != nil {
		log.Println("get_user_id_by_idfa msgpack:", err)
		return rv, err
	}

	return rv, err
}

func process_message(q chan []byte) {
	var app *eeyore.App
	var b []byte
	var ob []byte
	var err error

	for {

		b = <-q

		// log.Println(string(b))

		app = &eeyore.App{}
		err = eeyore.MsgpackUnpackb(b, app)
		if err != nil {
			log.Println("process_message msgpack:", err)
			continue
		}

		// log.Printf("%+v", app)

		apple_id := app.AppleId
		// log.Println("apple_id:", apple_id)

		mapping := make([]([]int64), 0)

		for k, v := range app.Result {
			idfa := k
			status := v

			user_id, _ := get_user_id_by_idfa(idfa)

			if user_id == 0 {
				log.Println(
					"cannot find user_id for idfa:", idfa)
				continue
			}

			var _v = make([]int64, 0)
			_v = append(_v, user_id)
			_v = append(_v, apple_id)
			_v = append(_v, int64(status))
			mapping = append(mapping, _v)
			write_check_result(user_id, apple_id, status)

			if status == 0 {
				logger.Info(fmt.Sprintf(
					"user %d has done %d",
					user_id, apple_id))
			}
			if status == 1 {
				logger.Info(fmt.Sprintf(
					"user %d has undone %d",
					user_id, apple_id))
			}
		}

		ob, err = eeyore.MsgpackPackb(mapping)
		if err != nil {
			continue
		}
		// log.Println("output bytes:", ob)
		push(ob)
	}

}

func main() {
	flag.Parse()
	config = eeyore.LoadConfig(*config_file)
	log.Printf("eeyore Config: %+v\n", config)

	initRedis()
	initKafkaLogger()

	q := make(chan []byte)
	for i := 0; i < *worker; i++ {
		go pop(q)
	}
	for i := 0; i < *worker; i++ {
		go process_message(q)
	}

	for {
		time.Sleep(time.Second * 10)
		log.Printf("processed: %d", cnt)
	}
}
