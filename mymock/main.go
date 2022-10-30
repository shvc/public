package main

import (
	"bytes"
	"fmt"
	"mymock/db"
	"mymock/dber"
)

func NewUser() *User {
	return &User{
		db: db.NewDB(),
	}
}

type User struct {
	db dber.Dber
}

func (s *User) Get(k string) ([]byte, error) {
	return s.db.Get(k)
}

func (s *User) Put(k string, v []byte) error {
	return s.db.Put(k, v)
}

func main() {
	u := NewUser()

	v10 := []byte("user-1")
	err := u.Put("1", v10)
	if err != nil {
		panic(err)
	}

	v11, err := u.Get("1")
	if err != nil {
		panic(err)
	}

	if !bytes.Equal(v10, v11) {
		fmt.Printf("error! expect:%s got:%s\n", v10, v11)
	} else {
		fmt.Printf("OK! got:%s\n", v11)
	}
}
