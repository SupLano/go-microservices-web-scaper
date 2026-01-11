package main

import (
	"context"
	"fmt"
	"log"

	// "time"

	"github.com/go-redis/redis/v8"
)

type RedisClient struct {
	client *redis.Client
}


func NewRedisClient(addr string) *RedisClient {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "", // no password set
		DB:       0,  // use default DB
	})



	// Test connection
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(pong)
	return &RedisClient{client: client}
}

func (r *RedisClient) CloseConnection() {	
	r.client.Close()
}

func (r *RedisClient) CheckAndMark(u string) bool {
	ctx := context.Background()
	exist := r.client.Exists(ctx, u).Val()
	if exist == 1 {
		return true
	}
	r.client.Set(ctx, u, "", 0)
	return false
}