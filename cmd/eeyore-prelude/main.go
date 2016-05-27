package main

import (
	"database/sql"
	"eeyore"
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/go-sql-driver/mysql"
	"gopkg.in/redis.v3"
)

var config eeyore.Config
var r *redis.Client
var db map[string]*sql.DB

func redis_connect() {
	if r == nil {
		r = redis.NewClient(&redis.Options{
			Addr: fmt.Sprintf(
				"%s:%d",
				config.Redis["backend"].Host,
				config.Redis["backend"].Port),
			Password: config.Redis["backend"].Pass,
			DB:       config.Redis["backend"].Db,
		})
	}
}

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

func cache_set(key string, m interface{}, expire time.Duration) error {
	redis_connect()

	b, err := eeyore.MsgpackPackb(m)

	if err != nil {
		return err
	}

	_, err = r.Set(key, b, expire).Result()

	if err != nil {
		r = nil
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
		cache_set(key, task, time.Minute*10)
	}
}

func loop() {
	tasks := get_todo_tasks()

	// log.Printf("%+v", tasks)
	err := cache_set("qianka:eeyore:todo_apps", tasks, time.Minute*5)

	if err != nil {
		log.Println(err)
	}

	cache_task_app_mapping(tasks)
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
