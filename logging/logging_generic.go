// +build !windows

package logging

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	color_map = map[string]string{
		"reset":  "\033[0m",
		"red":    "\033[31m",
		"green":  "\033[32m",
		"yellow": "\033[33m",
		"blue":   "\033[34m",
		"purple": "\033[35m",
		"cyan":   "\033[36m",
		"gray":   "\033[37m",
		"white":  "\033[97m",
	}
)

type Formatter struct {
	stderr_map lfshook.WriterMap
}

func (self *Formatter) Format(entry *logrus.Entry) ([]byte, error) {
	b := &bytes.Buffer{}

	levelText := strings.ToUpper(entry.Level.String())
	now := utils.GetTime().Now().UTC()

	fmt.Fprintf(b, "[%s] %v %s ", levelText, now.Format(time.RFC3339),
		replaceTagWithCode(strings.TrimRight(entry.Message, "\r\n")))

	if len(entry.Data) > 0 {
		serialized, _ := json.Marshal(entry.Data)
		fmt.Fprintf(b, "%s", serialized)
	}

	return append(b.Bytes(), '\n'), nil
}

func replaceTagWithCode(message string) string {
	if NoColor {
		return clearTag(message)
	}

	result := tag_regex.ReplaceAllStringFunc(message, func(hit string) string {
		matches := tag_regex.FindStringSubmatch(hit)
		if len(matches) > 1 {
			code, pres := color_map[matches[1]]
			if pres {
				return code
			}
		}
		return hit
	})

	reset := color_map["reset"]
	return closing_tag_regex.ReplaceAllString(result, reset) + reset
}
