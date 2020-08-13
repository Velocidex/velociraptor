package json

import "fmt"

func Debug(v interface{}) {
	fmt.Println(StringIndent(v))
}

func Dump(v interface{}) {
	fmt.Println(StringIndent(v))
}
