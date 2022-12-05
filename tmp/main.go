package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"time"
)

type Numberic interface {
	~int8 | ~int | ~int32 | ~int64 | ~uint8 | ~uint | ~uint32 | ~uint64 | ~float32 | ~float64
}

func Add[T Numberic](x, y T) T {
	return x + y
}

func checkExists(ctx context.Context, key string) (exists bool, err error) {
	result := make(chan bool, 1) // 必须有buffer,否则ctx先返回goroutine泄露
	go func() {
		time.Sleep(2 * time.Second)
		result <- true
	}()

	select {
	case <-ctx.Done():
		err = fmt.Errorf("ctx cancel %w", ctx.Err())
		return
	case exists = <-result:
		return
	}
}

func AnyoneExists(ctx context.Context, backendNum int, key string) (exists bool, err error) {
	ctx, cancFun := context.WithCancel(ctx)
	defer cancFun()
	existsChan := make(chan bool, backendNum)
	for i := 0; i < backendNum; i++ {
		go func() {
			ex, err := checkExists(ctx, key)
			if err != nil {
				return
			}
			existsChan <- ex
		}()
	}

	for i := 0; i < backendNum; i++ {
		select {
		case ex := <-existsChan:
			if ex {
				return ex, nil
			}
		case <-ctx.Done():
			return false, fmt.Errorf("done")
		}
	}
	return
}

func testAnyExists() {
	ctx := context.Background()
	ctx, cancFun := context.WithTimeout(ctx, 2*time.Second)
	defer cancFun()
	var i int
	for i = 0; i < 2; i++ {
		time.Sleep(1 * time.Second)
		ex, err := AnyoneExists(ctx, 3, "key")
		if err != nil {
			fmt.Printf("goroutine:%v, AnyoneExists error:%s\n", runtime.NumGoroutine(), err)
			continue
		}
		fmt.Printf("goroutine:%v, result:%v\n", runtime.NumGoroutine(), ex)
	}
	for i := 0; i < 1; i++ {
		time.Sleep(2 * time.Second)
		fmt.Printf("goroutine:%v\n", runtime.NumGoroutine())
	}
}

func WriteAll(ctx context.Context, dst []string, key string, r io.Reader) error {
	for i, v := range dst {
		fmt.Println(i, v)
		//io.TeeReader()
		pr, pw := io.Pipe()
		go func() {
			_, err := io.Copy(os.Stdout, pr)
			if err != nil {

			}
		}()
		io.Copy(pw, r)
	}
	io.MultiReader()

	return nil
}

func ReadAll(ctx context.Context, backendNum int, key string) error {

	return nil
}

func reflectUsage() {
	type cat struct {
		Name string
		Type int `json:"type" id:"100"`
	}
	c := cat{Name: "mimi", Type: 1}
	fmt.Println("value:", reflect.ValueOf(c))
	typeOfCat := reflect.TypeOf(c)
	fmt.Printf("name:%v, kind:%v\n", typeOfCat.Name(), typeOfCat.Kind()) // cat, struct
	for i := 0; i < typeOfCat.NumField(); i++ {
		catField := typeOfCat.Field(i)
		fmt.Printf("name:%v  tag:'%v'\n", catField.Name, catField.Tag)
	}

	if typeField, ok := typeOfCat.FieldByName("Type"); ok {
		fmt.Println("tag json:", typeField.Tag.Get("json"), "tag id:", typeField.Tag.Get("id"))
	}
}

func bubbleSort[T Numberic](a []T) {
	for i := 1; i < len(a); i++ {
		for j := 0; j < len(a)-i; j++ {
			if a[j] > a[j+1] {
				a[j], a[j+1] = a[j+1], a[j]
			}
		}
	}
}

func selectSort[T Numberic](a []T) {
	for i := 0; i < len(a)-1; i++ {
		for j := i + 1; j < len(a); j++ {
			if a[i] > a[j] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

func insertSort() {

}

func quickSort() {

}

func mergeSort() {

}

func heapSort() {

}

func myGrep1(infile, filter string) error {
	fd, err := os.Open(infile)
	if err != nil {
		return err
	}
	defer fd.Close()
	cmd := exec.Command("grep", filter)
	cmd.Stdin = fd
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func myGrep2(infile, filter string) error {
	fd, err := os.Open(infile)
	if err != nil {
		return err
	}
	defer fd.Close()
	cmd := exec.Command("grep", filter)
	pr, pw := io.Pipe()
	cmd.Stdin = pr
	go func() {
		io.Copy(pw, fd)
		pw.Close()
	}()

	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func main() {
	a := []int{4, 8, 7, 9, -1, 3, 5, 6, 1, 2, 0}
	//bubbleSort(a)
	selectSort(a)
	fmt.Println(a)

	fmt.Println("call myGrep1")
	if err := myGrep1("test.txt", "Hello"); err != nil {
		fmt.Println("myGrep1 error", err)
	}

	fmt.Println("call myGrep2")
	if err := myGrep2("test.txt", "Hello"); err != nil {
		fmt.Println("myGrep2 error", err)
	}
}
