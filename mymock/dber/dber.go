package dber

//go:generate mockgen -destination ../mocks/mock_db.go -package mocks mymock/dber Dber

type Dber interface {
	Get(key string) ([]byte, error)
	Put(key string, value []byte) error
}
