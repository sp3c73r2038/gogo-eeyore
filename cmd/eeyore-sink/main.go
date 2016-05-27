package main

import (
	"database/sql"
	"eeyore"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"strings"
	"time"
)

import (
	"github.com/garyburd/redigo/redis"
	"github.com/go-sql-driver/mysql"
	"github.com/ugorji/go/codec"
)

var (
	configFile = flag.String("config", "eeyore.toml", "config file")
	worker     = flag.Int("worker", 16, "worker number")
)

var config eeyore.Config
var r map[string]*redis.Pool
var db map[string]*sql.DB
var cnt int64

func logError(err error) {
	if err != nil {
		log.Println(err)
	}
}

func panicError(err error) {
	if err != nil {
		log.Panic(err)
	}
}

func mysqlConnect(bind string) error {
	var rv error
	if _, ok := db[bind]; ok {
		return rv
	}

	cfg := config.Database[bind]
	mysql_config := mysql.Config{
		Net:    "tcp",
		User:   cfg.User,
		Passwd: cfg.Pass,
		Addr:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		DBName: cfg.Db,
	}
	dsn := mysql_config.FormatDSN()

	// log.Println(dsn)

	con, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	err = con.Ping()
	if err != nil {
		return err
	}

	con.SetMaxOpenConns(*worker)
	con.SetMaxIdleConns(0)

	db[bind] = con
	return err
}

func getDb(bind string) *sql.DB {
	if db == nil {
		db = make(map[string]*sql.DB)
	}

	if _, ok := db[bind]; ok {
		return db[bind]
	}

	err := mysqlConnect(bind)
	logError(err)
	if err != nil {
		return nil
	}

	return db[bind]
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

	r["backend"] = newRedisPool(config.Redis["backend"])
}

func pop() []([]int64) {
	rv := make([]([]int64), 0)
	client := r["backend"].Get()
	defer client.Close()

	info, err := client.Do("BLPOP",
		"qianka:eeyore:pending_write_back", 5)

	if err != nil {
		// r["backend"] = nil
		log.Printf("pop error: %+v", err)
		return rv
	} else if info == nil {
		// log.Println("no pending payload")
		return rv
	}

	cnt++

	payload := info.([]interface{})[1].([]byte)

	var h codec.MsgpackHandle
	var dec *codec.Decoder = codec.NewDecoderBytes(payload, &h)
	err = dec.Decode(&rv)
	// log.Printf("payload: %+v", rv)

	return rv
}

func writeBack(mapping []([]int64)) {
	_value := make([]string, 0)

	for _, m := range mapping {
		_row := fmt.Sprintf("(%d, %d, %d)", m[0], m[1], m[2])
		_value = append(_value, _row)
	}
	values := strings.Join(_value, ", ")

	now := time.Now()

	sql := fmt.Sprintf(
		`INSERT INTO user_app_status_%s (user_id, app_id, status)
VALUES %s
ON DUPLICATE KEY UPDATE status=values(status), updated_at = NOW()
`, now.Format("20060102"), values)

	// log.Println(sql)

	con := getDb("idfa_check")

	if con == nil {
		return
	}

	stmt, err := con.Prepare(sql)
	defer stmt.Close()
	logError(err)
	if err != nil {
		return
	}

	res, err := stmt.Exec()
	logError(err)
	if err != nil {
		return
	}

	_, err = res.RowsAffected()
	logError(err)
	// log.Println("updated rows:", rowCnt)
}

func sinkWorker() {
	for {
		payload := pop()
		if len(payload) <= 0 {
			continue
		}
		writeBack(payload)
	}
}

func main() {
	flag.Parse()
	config = eeyore.LoadConfig(*configFile)
	log.Printf("%+v", config)
	log.Println("available GOMAXPROCS:", runtime.GOMAXPROCS(*worker))
	initRedis()

	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}()

	for i := 0; i < *worker; i++ {
		go sinkWorker()
	}

	for {
		time.Sleep(time.Second * 5)
		log.Println("processed:", cnt)
	}
}
