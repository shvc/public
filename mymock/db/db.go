package db

import (
	"fmt"
	"sync"
)

func NewDB() *DB {
	return &DB{
		stor: map[string][]byte{},
	}
}

type DB struct {
	sync.RWMutex
	stor map[string][]byte
}

func (d *DB) Get(key string) (v []byte, e error) {
	d.RLock()
	defer d.RUnlock()
	v, ok := d.stor[key]
	if !ok {
		e = fmt.Errorf("not exists")
	}
	return
}

func (d *DB) Put(key string, v []byte) error {
	if key == "" {
		return fmt.Errorf("empty key")
	}

	if v == nil {
		return fmt.Errorf("empty value")
	}

	d.Lock()
	defer d.Unlock()
	d.stor[key] = v
	return nil
}
