package main

import (
	"database/sql"
	"eeyore"
	"flag"
	"fmt"
	"log"
	"regexp"
	"runtime"
	"time"
)

import (
	"github.com/go-sql-driver/mysql"
)

var (
	config_file = flag.String("config", "eeyore.toml", "config file")
	worker      = flag.Int("worker", 4, "worker number")
)

var config eeyore.Config
var db map[string]*sql.DB
var r map[string]*eeyore.RedisPool

func mysql_connect(bind string) {

	if _, ok := db[bind]; ok {
		return
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
		panic(err)
	}

	err = con.Ping()

	if err != nil {
		panic(err)
	}

	db[bind] = con

}

func get_db(bind string) *sql.DB {
	if db == nil {
		db = make(map[string]*sql.DB)
	}

	if _, ok := db[bind]; ok {
		return db[bind]
	}

	mysql_connect(bind)
	return db[bind]
}

func cache_set(key string, m interface{}, expire int) error {

	b, err := eeyore.MsgpackPackb(m)

	if err != nil {
		return err
	}

	client := r["backend"].Get()
	defer client.Close()

	_, err = client.Do("SETEX", key, expire, b)

	if err != nil {
		r = nil
		log.Printf("pop: %T\n", err)
		log.Printf("pop: %+v\n", err)
		r = eeyore.InitRedis(*worker, []string{"backend"}, config)
	}

	return err
}

func pop() {
}

func push() {
}

func get_todo_tasks() map[int64]eeyore.App {

	var rv map[int64]eeyore.App = make(map[int64]eeyore.App)

	sql := `SELECT
id, ad_id, idfa_url, appid, idfa_type,
sum(subsurplus - subowntask) as left_
FROM
subtasks
WHERE
substop = 0 AND idfa_url != ''
AND subbundleid != '' AND idfa_type >= 0
AND check_status = 1
GROUP BY idfa_url, appid
HAVING left_ > 0
ORDER BY left_ DESC
-- golang eeyore-prelude
`
	con := get_db("zhuanqian")

	rows, err := con.Query(sql)

	if err != nil {
		log.Println("query mysql error:", err)
	}

	var id int64
	var ad_id int64
	var idfa_url string
	var apple_id int64
	var idfa_type int
	var left int

	for rows.Next() {
		err := rows.Scan(
			&id,
			&ad_id,
			&idfa_url,
			&apple_id,
			&idfa_type,
			&left,
		)

		m, err := regexp.MatchString(`https?://.+`, idfa_url)
		if !m {
			continue
		}

		rv[apple_id] = eeyore.App{
			AdvertiserId: ad_id,
			Url:          idfa_url,
			AppleId:      apple_id,
			IdfaType:     idfa_type,
		}

		if err != nil {
			log.Println("query mysql error:", err)
			continue
		}
	}

	return rv
}

func cache_task_app_mapping(tasks map[int64]eeyore.App) {

	var key string
	for _, task := range tasks {
		// log.Printf("%+v", task)
		key = fmt.Sprintf("qianka:eeyore:app_%d", task.AppleId)

		cache_set(key, task, 600) // 10 minutes
	}
}

func loop() {
	tasks := get_todo_tasks()

	// log.Printf("%+v", tasks)
	err := cache_set("qianka:eeyore:todo_apps", tasks, 300) // 5 minutes

	if err != nil {
		log.Println(err)
	}

	cache_task_app_mapping(tasks)
}

func main() {

	flag.Parse()
	config = eeyore.LoadConfig(*config_file)
	log.Printf("eeyore Config: %+v\n", config)
	log.Println("available GOMAXPROCS:", runtime.GOMAXPROCS(*worker))

	r = eeyore.InitRedis(*worker, []string{"backend"}, config)

	cnt := 0
	for {
		if cnt%10 == 0 {
			log.Printf("have done %d loops", cnt)
		}
		loop()
		cnt++
		time.Sleep(time.Second * 30)
	}
}
