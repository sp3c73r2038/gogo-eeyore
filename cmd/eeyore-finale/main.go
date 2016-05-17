package main

import (
	"eeyore"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ugorji/go/codec"
	"gopkg.in/redis.v3"
)

var config eeyore.Config
var r map[string]*redis.Client

func get_redis(bind string) {
	client := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf(
			"%s:%d",
			config.Redis[bind].Host,
			config.Redis[bind].Port),
		Password: config.Redis[bind].Pass,
		DB:       config.Redis[bind].Db,
	})
	return client
}

func write_check_result(user_id int64, apple_id int64, status int) error {
	key := fmt.Sprintf("hera:idfa-check:result_%d_%d", user_id, apple_id)
	value := fmt.Sprintf("%d %d", status, int(time.Now().Unix()))

	client := get_redis("result")
	defer client.Close()

	_, err := client.Set(key, value, time.Second*0).Result()
	return err
}

// pop from pending_cache queue
// * include msgpack unpack
func pop_worker(q chan []byte) {
	client := get_redis("backend")
	defer client.Close()

	for {
		info, err := client.BLPop(
			time.Second*5, "qianka:eeyore:pending_cache").Result()

		if err == redis.Nil {
			log.Println("no pending payload")
			continue
		} else if err != nil {
			// r["backend"] = nil
			log.Printf("pop error: %+v", err)
			continue
		}

		q <- []byte(info[1])
	}
}

// push to pending_cache queue
func push(b []byte) {
	var err error

	client := get_redis("backend")
	defer client.Close()

	_, err = client.RPush(
		"qianka:eeyore:pending_write_back", string(b)).Result()

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

	client := get_redis("backend")

	info, err := client.Get(key).Result()

	if err != nil {
		log.Println("get_user_id_by_idfa redis:", err)
		return rv, err
	}

	var h codec.MsgpackHandle

	var dec *codec.Decoder = codec.NewDecoderBytes([]byte(info), &h)
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
		log.Printf("%+v", apple_id)

		for k, v := range app.Result {
			idfa := k
			status := v

			user_id, _ := get_user_id_by_idfa(idfa)

			if user_id == 0 {
				log.Println(
					"cannot find user_id for idfa:", idfa)
				continue
			}

			write_check_result(user_id, apple_id, status)
		}

		push(b)
	}

}

func main() {
	config_filename := ""

	if len(os.Args) == 1 {
		config_filename = "eeyore.toml"
	} else {
		config_filename = os.Args[1]
	}

	config = eeyore.LoadConfig(config_filename)
	log.Printf("eeyore Config: %+v\n", config)

	q := make(chan []byte)
	for i := 0; i < 16; i++ {
		go pop(q)
	}
	for i := 0; i < 32; i++ {
		go process_message(q)
	}

	for {
		time.Sleep(time.Second * 1)
	}
}
