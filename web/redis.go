package main

import (
	"errors"
	"fmt"
	"github.com/Terry-Mao/gopush-cluster/hash"
	"github.com/garyburd/redigo/redis"
	"time"
)

const (
	defaultRedisNode = "node1"
)

var (
	RedisZADDReplyErr = errors.New("zadd fail, reply != 1")
	RedisNoConnErr    = errors.New("can't get a redis conn")
	redisPool         = map[string]*redis.Pool{}
	redisHash         *hash.Ketama
)

// Initialize redis pool, Initialize consistent hash ring
func InitRedis() {
	// Redis pool
	for n, c := range Conf.Redis {
		// WARN: closures use
		tc := c
		redisPool[n] = &redis.Pool{
			MaxIdle:     tc.MaxIdle,
			MaxActive:   tc.MaxActive,
			IdleTimeout: time.Duration(tc.IdleTimeout) * time.Second,
			Dial: func() (redis.Conn, error) {
				conn, err := redis.Dial(tc.Network, tc.Addr)
				if err != nil {
					Log.Error("redis.Dial(\"%s\", \"%s\") failed (%v)", tc.Network, tc.Addr, err)
				}
				return conn, err
			},
		}
	}

	// Consistent hashing
	redisHash = hash.NewKetama(len(redisPool), 255)
}

// SaveMessage save offline messages
func SaveMessage(key, msg string, mid int64) error {
	conn := getRedisConn(key)
	if conn == nil {
		return RedisNoConnErr
	}

	defer conn.Close()
	reply, err := redis.Int(conn.Do("ZADD", key, mid, msg))
	if err != nil {
		return err
	}

	if reply != 1 {
		return RedisZADDReplyErr
	}

	return nil
}

// GetMessages et all of offline messages which larger than mid
func GetMessages(key string, mid int64) ([]string, error) {
	conn := getRedisConn(key)
	if conn == nil {
		return nil, RedisNoConnErr
	}

	defer conn.Close()
	reply, err := redis.Strings(conn.Do("ZRANGEBYSCORE", key, fmt.Sprintf("(%d", mid), "+inf"))
	if err != nil {
		if err == redis.ErrNil {
			return nil, nil
		}

		return nil, err
	}

	return reply, nil
}

// getRedisConn get the redis connection of matching with key
func getRedisConn(key string) redis.Conn {
	node := defaultRedisNode
	// if multiple redispool use ketama
	if len(redisPool) != 1 {
		node = redisHash.Node(key)
	}

	p, ok := redisPool[node]
	if !ok {
		Log.Warn("no exists key:%s in redisPool map", key)
		return nil
	}

	Log.Debug("key :%s, node : %s", key, node)
	return p.Get()
}
