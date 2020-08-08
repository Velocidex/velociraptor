package json

import "fmt"

func Debug(v interface{}) {
	fmt.Println(string(MustMarshalIndent(v)))
}
