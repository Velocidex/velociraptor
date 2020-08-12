// +build windows

package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gookit/color"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
)

type Formatter struct {
	stderr_map lfshook.WriterMap
}

func (self *Formatter) Format(entry *logrus.Entry) ([]byte, error) {
	b := &bytes.Buffer{}

	levelText := strings.ToUpper(entry.Level.String())
	fmt.Fprintf(b, "[%s] %v %s ", levelText, entry.Time.Format(time.RFC3339),
		strings.TrimRight(entry.Message, "\r\n"))

	if len(entry.Data) > 0 {
		serialized, _ := json.Marshal(entry.Data)
		fmt.Fprintf(b, "%s", serialized)
	}

	// Only print the result to the console, if there is an stderr
	// map to it.
	_, pres := self.stderr_map[entry.Level]
	if pres {
		if NoColor {
			return []byte(clearTag(b.String())), nil
		}
		color.Println(normalize(b.String()))
	}

	return nil, nil
}

func normalize(line string) string {
	// Get count of opening tags
	opening_matches := tag_regex.FindAllString(line, -1)
	closing_matches := closing_tag_regex.FindAllString(line, -1)

	if len(opening_matches) > len(closing_matches) {
		for i := 0; i < len(opening_matches)-len(closing_matches); i++ {
			line += "</>"
		}
	} else if len(opening_matches) < len(closing_matches) {
		line = closing_tag_regex.ReplaceAllString(line, "")
	}

	return line
}
