package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache Redis 缓存封装，提供 Agent 和 Plugin 共用的缓存接口。
type Cache struct {
	client *redis.Client
	prefix string
}

func NewCache(client *redis.Client, prefix string) *Cache {
	if prefix == "" {
		prefix = "juan:"
	}
	return &Cache{client: client, prefix: prefix}
}

func (c *Cache) key(k string) string { return c.prefix + k }

// Client 返回底层 Redis 客户端，供需要高级操作（如 PubSub）的模块使用。
func (c *Cache) Client() *redis.Client { return c.client }

// ---------- 基础 KV ----------

func (c *Cache) Get(ctx context.Context, key string, dest any) error {
	data, err := c.client.Get(ctx, c.key(key)).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (c *Cache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.key(key), data, ttl).Err()
}

func (c *Cache) Del(ctx context.Context, keys ...string) error {
	full := make([]string, len(keys))
	for i, k := range keys {
		full[i] = c.key(k)
	}
	return c.client.Del(ctx, full...).Err()
}

func (c *Cache) Exists(ctx context.Context, keys ...string) (int64, error) {
	full := make([]string, len(keys))
	for i, k := range keys {
		full[i] = c.key(k)
	}
	return c.client.Exists(ctx, full...).Result()
}

// SetNX 仅当 key 不存在时设置，返回 true 表示设置成功。
func (c *Cache) SetNX(ctx context.Context, key string, val any, ttl time.Duration) (bool, error) {
	data, err := json.Marshal(val)
	if err != nil {
		return false, err
	}
	return c.client.SetNX(ctx, c.key(key), data, ttl).Result()
}

// ---------- List 操作 (短期记忆滑动窗口) ----------

func (c *Cache) LPush(ctx context.Context, key string, vals ...any) error {
	full := make([]any, len(vals))
	for i, v := range vals {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		full[i] = data
	}
	return c.client.LPush(ctx, c.key(key), full...).Err()
}

func (c *Cache) RPush(ctx context.Context, key string, vals ...any) error {
	full := make([]any, len(vals))
	for i, v := range vals {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		full[i] = data
	}
	return c.client.RPush(ctx, c.key(key), full...).Err()
}

func (c *Cache) LRange(ctx context.Context, key string, start, stop int64, dest any) error {
	results, err := c.client.LRange(ctx, c.key(key), start, stop).Result()
	if err != nil {
		return err
	}

	items := make([]json.RawMessage, 0, len(results))
	for _, r := range results {
		items = append(items, json.RawMessage(r))
	}
	raw, _ := json.Marshal(items)
	return json.Unmarshal(raw, dest)
}

func (c *Cache) LTrim(ctx context.Context, key string, start, stop int64) error {
	return c.client.LTrim(ctx, c.key(key), start, stop).Err()
}

func (c *Cache) LLen(ctx context.Context, key string) (int64, error) {
	return c.client.LLen(ctx, c.key(key)).Result()
}

// ---------- PubSub (任务结果通知) ----------

func (c *Cache) Publish(ctx context.Context, channel string, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.client.Publish(ctx, c.key(channel), data).Err()
}

func (c *Cache) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	full := make([]string, len(channels))
	for i, ch := range channels {
		full[i] = c.key(ch)
	}
	return c.client.Subscribe(ctx, full...)
}

// ---------- Hash 操作 ----------

func (c *Cache) HGet(ctx context.Context, key, field string, dest any) error {
	data, err := c.client.HGet(ctx, c.key(key), field).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (c *Cache) HSet(ctx context.Context, key, field string, val any) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return c.client.HSet(ctx, c.key(key), field, data).Err()
}

func (c *Cache) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, c.key(key)).Result()
}

func (c *Cache) HDel(ctx context.Context, key string, fields ...string) error {
	return c.client.HDel(ctx, c.key(key), fields...).Err()
}
