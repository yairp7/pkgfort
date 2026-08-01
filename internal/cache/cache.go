package cache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Entry holds arbitrary metadata fields for a cached package version.
// Values are raw JSON so types survive round-trips through disk.
type Entry map[string]json.RawMessage

type Cache struct {
	mu      sync.RWMutex
	entries map[string]Entry
	path    string
}

func New(path string) (*Cache, error) {
	c := &Cache{
		entries: make(map[string]Entry),
		path:    path,
	}
	if err := c.load(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Cache) Get(key string) (Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	return e, ok
}

func (c *Cache) Set(ctx context.Context, key string, e Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = e
	return c.save()
}

func (c *Cache) load() error {
	f, err := os.Open(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(&c.entries)
}

// save writes the cache atomically via a temp file + rename.
func (c *Cache) save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(c.entries); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, c.path)
}
