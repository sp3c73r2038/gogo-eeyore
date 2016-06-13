package eeyore

import (
	"fmt"
	"time"
)

import (
	"github.com/garyburd/redigo/redis"
)

type RedisPool struct {
	Pool redis.Pool
	Db   int64
}

func (p *RedisPool) Get() redis.Conn {
	conn := p.Pool.Get()
	conn.Do("SELECT", p.Db)
	return conn
}

func newRedisPool(worker int, server Redis) *RedisPool {
	return &RedisPool{
		Pool: redis.Pool{
			MaxActive:   worker * 2,
			MaxIdle:     worker,
			IdleTimeout: 5 * time.Second,
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
		},
		Db: server.Db,
	}
}

func InitRedis(
	worker int, binds []string, config Config) map[string]*RedisPool {
	var r map[string]*RedisPool
	r = make(map[string]*RedisPool)

	for i := 0; i < len(binds); i++ {
		bind := binds[i]
		r[bind] = newRedisPool(worker, config.Redis[bind])
	}
	return r
}
