package server

import (
	"mymock/db"
	"mymock/dber"
)

func NewServer() *Server {
	return &Server{
		db: db.NewDB(),
	}
}

type Server struct {
	db dber.Dber
}

func (s *Server) Get(k string) ([]byte, error) {
	return s.db.Get(k)
}

func (s *Server) Put(k string, v []byte) error {
	return s.db.Put(k, v)
}
