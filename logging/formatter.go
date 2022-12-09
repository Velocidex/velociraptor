package logging

import (
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/utils"
)

type JSONFormatter struct {
	*logrus.JSONFormatter
}

func (self *JSONFormatter) Format(e *logrus.Entry) ([]byte, error) {
	e.Time = utils.GetTime().Now().UTC()

	result, err := self.JSONFormatter.Format(e)
	return result, err
}
