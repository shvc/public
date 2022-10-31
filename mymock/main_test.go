package main

import (
	"errors"
	"fmt"
	"mymock/mocks"
	"testing"

	"github.com/golang/mock/gomock"
)

func TestGet(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	mdb := mocks.NewMockDber(ctl)
	svc := User{
		db: mdb,
	}

	mdb.EXPECT().Get("x").Return(nil, errors.New("not exists")).Times(2)
	svc.Get("x")
	svc.Get("x")

}

func TestPut(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	mdb := mocks.NewMockDber(ctl)
	svc := User{
		db: mdb,
	}

	mdb.EXPECT().Put("x", []byte("XX")).Return(nil).MinTimes(1)
	mdb.EXPECT().Put("", gomock.Any()).Return(fmt.Errorf("empty key")).Times(1)
	mdb.EXPECT().Put(gomock.Any(), nil).Return(fmt.Errorf("empty value")).Times(1)
	svc.Put("x", []byte("XX"))
	svc.Put("", []byte("M"))
	svc.Put("xx", nil)

}

func TestPutGet(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	mdb := mocks.NewMockDber(ctl)
	svc := User{
		db: mdb,
	}

	cput := mdb.EXPECT().Put("x", []byte("XX")).Return(nil).Times(1)
	mdb.EXPECT().Get("x").Return([]byte("XX"), nil).After(cput).Times(1)
	svc.Put("x", []byte("XX"))
	svc.Get("x")
}

func TestPutsGets(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	mdb := mocks.NewMockDber(ctl)
	svc := User{
		db: mdb,
	}

	vx := []byte("XX")
	vy := []byte("ValueY")

	gomock.InOrder(
		mdb.EXPECT().Put("x", vx).Return(nil),
		mdb.EXPECT().Get("x").Return(vx, nil),
		mdb.EXPECT().Put("y", vy).Return(nil),
		mdb.EXPECT().Get("y").Return(vy, nil),
	)
	svc.Put("x", vx)
	svc.Get("x")

	svc.Put("y", vy)
	svc.Get("y")
}
