// Copyright 2013 Beego Authors
// Copyright 2014 Unknwon
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package cache

import (
	"crypto/md5"
	"encoding/hex"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Unknwon/com"
	"github.com/Unknwon/macaron"
)

// Item represents a cache item.
type Item struct {
	Val     interface{}
	Created int64
	Expire  int64
}

// FileCacher represents a file cache adapter implementation.
type FileCacher struct {
	rootPath string
	interval int // GC interval.
}

// NewFileCacher creates and returns a new file cacher.
func NewFileCacher() *FileCacher {
	return &FileCacher{}
}

func (c *FileCacher) filepath(key string) string {
	m := md5.Sum([]byte(key))
	hash := hex.EncodeToString(m[:])
	return filepath.Join(c.rootPath, string(hash[0]), string(hash[1]), hash)
}

// Put puts value into cache with key and expire time.
// If expired is 0, it will be deleted by next GC operation.
func (c *FileCacher) Put(key string, val interface{}, expire int64) error {
	filename := c.filepath(key)
	item := &Item{val, time.Now().Unix(), expire}
	data, err := EncodeGob(item)
	if err != nil {
		return err
	}

	os.MkdirAll(filepath.Dir(filename), os.ModePerm)
	return ioutil.WriteFile(filename, data, os.ModePerm)
}

func (c *FileCacher) read(key string) (*Item, error) {
	filename := c.filepath(key)

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	item := new(Item)
	return item, DecodeGob(data, item)
}

// Get gets cached value by given key.
func (c *FileCacher) Get(key string) interface{} {
	item, err := c.read(key)
	if err != nil {
		return nil
	}

	if item.Expire > 0 &&
		(time.Now().Unix()-item.Created) >= item.Expire {
		os.Remove(c.filepath(key))
		return nil
	}
	return item.Val
}

// Delete deletes cached value by given key.
func (c *FileCacher) Delete(key string) error {
	return os.Remove(c.filepath(key))
}

// Incr increases cached int-type value by given key as a counter.
func (c *FileCacher) Incr(key string) error {
	item, err := c.read(key)
	if err != nil {
		return err
	}

	item.Val, err = Incr(item.Val)
	if err != nil {
		return err
	}

	return c.Put(key, item.Val, item.Expire)
}

// Decrease cached int value.
func (c *FileCacher) Decr(key string) error {
	item, err := c.read(key)
	if err != nil {
		return err
	}

	item.Val, err = Decr(item.Val)
	if err != nil {
		return err
	}

	return c.Put(key, item.Val, item.Expire)
}

// IsExist returns true if cached value exists.
func (c *FileCacher) IsExist(key string) bool {
	return com.IsExist(c.filepath(key))
}

// Flush deletes all cached data.
func (c *FileCacher) Flush() error {
	return os.RemoveAll(c.rootPath)
}

func (c *FileCacher) startGC() {
	if c.interval < 1 {
		return
	}

	if err := filepath.Walk(c.rootPath, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		data, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		item := new(Item)
		if err = DecodeGob(data, item); err != nil {
			return err
		}
		if (time.Now().Unix() - item.Created) >= item.Expire {
			return os.Remove(path)
		}
		return nil
	}); err != nil {
		log.Printf("error garbage collecting cache files: %v", err)
	}

	time.AfterFunc(time.Duration(c.interval)*time.Second, func() { c.startGC() })
}

// StartAndGC starts GC routine based on config string settings.
func (c *FileCacher) StartAndGC(opt Options) error {
	c.rootPath = opt.AdapterConfig

	if !filepath.IsAbs(c.rootPath) {
		c.rootPath = filepath.Join(macaron.Root, c.rootPath)
	}

	if err := os.MkdirAll(c.rootPath, os.ModePerm); err != nil {
		return err
	}

	go c.startGC()
	return nil
}

func init() {
	Register("file", NewFileCacher())
}