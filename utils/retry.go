package utils

import "time"

func Retry(cb func() error, number int, sleep time.Duration) error {
	var err error
	for i := 0; i < number; i++ {
		err = cb()
		if err == nil {
			return err
		}
		time.Sleep(sleep)
	}
	return err
}
