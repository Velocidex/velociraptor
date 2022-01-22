/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package logging

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	rotatelogs "github.com/Velocidex/file-rotatelogs"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	SuppressLogging = false
	NoColor         = false

	GenericComponent  = "Velociraptor"
	FrontendComponent = "VelociraptorFrontend"
	ClientComponent   = "VelociraptorClient"
	GUIComponent      = "VelociraptorGUI"
	ToolComponent     = "Velociraptor"
	APICmponent       = "VelociraptorAPI"

	// Used for high value audit related events.
	Audit = "VelociraptorAudit"

	// Lock for log manager.
	mu        sync.Mutex
	Manager   *LogManager
	node_name = ""

	// Lock for memory logs and prelogs.
	memory_log_mu sync.Mutex
	prelogs       []string
	memory_logs   []string

	tag_regex         = regexp.MustCompile("<([^>/0]+)>")
	closing_tag_regex = regexp.MustCompile("</>")
)

func SetNodeName(name string) {
	mu.Lock()
	defer mu.Unlock()

	node_name = name
}

func GetNodeName() string {
	mu.Lock()
	defer mu.Unlock()

	return node_name
}

func InitLogging(config_obj *config_proto.Config) error {
	mu.Lock()
	Manager = &LogManager{
		contexts: make(map[*string]*LogContext),
	}

	for _, component := range []*string{&GenericComponent,
		&FrontendComponent, &ClientComponent,
		&GUIComponent, &ToolComponent, &APICmponent, &Audit} {

		logger, err := Manager.makeNewComponent(config_obj, component)
		if err != nil {
			return err
		}
		Manager.contexts[component] = logger
	}
	mu.Unlock()

	FlushPrelogs(config_obj)

	return nil
}

func ClearMemoryLogs() {
	memory_logs = nil
}

func GetMemoryLogs() []string {
	memory_log_mu.Lock()
	defer memory_log_mu.Unlock()

	return append([]string{}, memory_logs...)
}

// Early in the startup process, we find that we need to log sometimes
// but we have no idea where to send the logs and what components to
// load (because the config is not fully loaded yet). We therefore
// queue these messages until we are able to flush them.
func Prelog(format string, v ...interface{}) {
	memory_log_mu.Lock()
	defer memory_log_mu.Unlock()

	// Truncate too many logs
	if len(prelogs) > 1000 {
		prelogs = nil
	}

	prelogs = append(prelogs, fmt.Sprintf(format, v...))
}

func FlushPrelogs(config_obj *config_proto.Config) {
	logger := GetLogger(config_obj, &GenericComponent)

	memory_log_mu.Lock()
	lprelogs := append([]string{}, prelogs...)
	memory_log_mu.Unlock()

	for _, msg := range lprelogs {
		logger.Info(msg)
	}
	prelogs = make([]string, 0)
}

type LogContext struct {
	*logrus.Logger
}

func (self *LogContext) Debug(format string, v ...interface{}) {
	if self.Logger != nil {
		self.Logger.Debug(fmt.Sprintf(format, v...))
	}
}

func (self *LogContext) Info(format string, v ...interface{}) {
	if self.Logger != nil {
		self.Logger.Info(fmt.Sprintf(format, v...))
	}
}

func (self *LogContext) Warn(format string, v ...interface{}) {
	if self.Logger != nil {
		self.Logger.Warn(fmt.Sprintf(format, v...))
	}
}

func (self *LogContext) Error(format string, v ...interface{}) {
	if self.Logger != nil {
		self.Logger.Error(fmt.Sprintf(format, v...))
	}
}

type LogManager struct {
	mu       sync.Mutex
	contexts map[*string]*LogContext
}

// Get the logger from cache - creating it if it needs to.
func (self *LogManager) GetLogger(
	config_obj *config_proto.Config,
	component *string) *LogContext {
	if config_obj == nil {
		return nil
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	if config_obj.Logging != nil &&
		!config_obj.Logging.SeparateLogsPerComponent {
		component = &GenericComponent
	}

	ctx, pres := self.contexts[component]
	if !pres {
		panic(fmt.Sprintf("Uninitialized logging for %v", *component))
	}
	return ctx
}

func (self *LogManager) Reset() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.contexts = make(map[*string]*LogContext)
}

func Reset() {
	if Manager != nil {
		Manager.Reset()
	}
}

func getRotator(
	config_obj *config_proto.Config,
	rotator_config *config_proto.LoggingRetentionConfig,
	base_path string) (io.Writer, error) {

	if rotator_config == nil {
		rotator_config = &config_proto.LoggingRetentionConfig{
			RotationTime: config_obj.Logging.RotationTime,
			MaxAge:       config_obj.Logging.MaxAge,
		}
	}

	if rotator_config.Disabled {
		return ioutil.Discard, nil
	}

	max_age := rotator_config.MaxAge
	if max_age == 0 {
		max_age = 86400 * 365 // 1 year.
	}

	rotation := rotator_config.RotationTime
	if rotation == 0 {
		rotation = 604800 // 7 days
	}

	result, err := rotatelogs.New(
		base_path+".%Y%m%d%H%M",
		rotatelogs.WithLinkName(base_path),
		// 365 days.
		rotatelogs.WithMaxAge(time.Duration(max_age)*time.Second),
		// 7 days.
		rotatelogs.WithRotationTime(time.Duration(rotation)*time.Second),
	)
	if err != nil {
		return nil, err
	}

	// Make sure to write one message to confirm that we can actually
	// write to the file.
	_, err = result.Write([]byte("Starting...\n"))
	return result, err
}

func (self *LogManager) makeNewComponent(
	config_obj *config_proto.Config,
	component *string) (*LogContext, error) {

	Log := logrus.New()
	Log.Out = inMemoryLogWriter{}
	Log.Level = logrus.DebugLevel

	if config_obj != nil && config_obj.Logging != nil &&
		config_obj.Logging.OutputDirectory != "" {

		base_directory := filepath.Join(
			config_obj.Logging.OutputDirectory, GetNodeName(node_name))
		err := os.MkdirAll(base_directory, 0700)
		if err != nil {
			return nil, errors.New("Unable to create logging directory.")
		}

		base_filename := filepath.Join(base_directory, *component)
		pathMap := lfshook.WriterMap{}

		rotator, err := getRotator(
			config_obj, config_obj.Logging.Debug,
			base_filename+"_debug.log")
		if err != nil {
			return nil, err
		}
		pathMap[logrus.DebugLevel] = rotator

		rotator, err = getRotator(
			config_obj, config_obj.Logging.Info,
			base_filename+"_info.log")
		if err != nil {
			return nil, err
		}
		pathMap[logrus.InfoLevel] = rotator

		rotator, err = getRotator(
			config_obj, config_obj.Logging.Error,
			base_filename+"_error.log")
		if err != nil {
			return nil, err
		}
		pathMap[logrus.ErrorLevel] = rotator

		hook := lfshook.NewHook(
			pathMap,
			&logrus.JSONFormatter{
				DisableHTMLEscape: true,
			},
		)
		Log.Hooks.Add(hook)
	}

	// Add stderr logging if required.
	stderr_map := lfshook.WriterMap{
		logrus.ErrorLevel: os.Stderr,
	}

	if !SuppressLogging {
		stderr_map[logrus.DebugLevel] = os.Stderr
		stderr_map[logrus.InfoLevel] = os.Stderr
		stderr_map[logrus.WarnLevel] = os.Stderr
		stderr_map[logrus.ErrorLevel] = os.Stderr
	}

	Log.Hooks.Add(lfshook.NewHook(stderr_map, &Formatter{stderr_map}))
	if !NoColor && !isatty.IsTerminal(os.Stdout.Fd()) {
		NoColor = true
	}

	return &LogContext{Log}, nil
}

func AddLogFile(filename string) error {
	fd, err := os.OpenFile(filename,
		os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}

	writer_map := lfshook.WriterMap{
		logrus.ErrorLevel: fd,
		logrus.DebugLevel: fd,
		logrus.InfoLevel:  fd,
		logrus.WarnLevel:  fd,
	}

	for _, log := range Manager.contexts {
		log.Hooks.Add(lfshook.NewHook(
			writer_map, &logrus.JSONFormatter{
				DisableHTMLEscape: true,
			},
		))
	}
	return nil
}

type logWriter struct {
	logger *LogContext
}

func (self *logWriter) Write(b []byte) (int, error) {
	self.logger.Info("%s", string(b))
	return len(b), nil
}

// A log compatible logger.
func NewPlainLogger(
	config *config_proto.Config,
	component *string) *log.Logger {
	if !SuppressLogging {
		return log.New(&logWriter{
			GetLogger(config, component)}, "", 0)
	}

	return log.New(ioutil.Discard, "", 0)
}

func GetLogger(config_obj *config_proto.Config, component *string) *LogContext {
	mu.Lock()
	lManager := Manager
	mu.Unlock()

	if lManager == nil {
		err := InitLogging(config_obj)
		if err != nil {
			panic(err)
		}
	}
	return Manager.GetLogger(config_obj, component)
}

type stackTracer interface {
	StackTrace() errors.StackTrace
}

func GetStackTrace(err error) string {
	if err, ok := err.(stackTracer); ok {
		for _, f := range err.StackTrace() {
			return fmt.Sprintf("%+s:%d\n", f, f)
		}
	}
	return ""
}

// Clear tags from log messages.
func clearTag(message string) string {
	message = tag_regex.ReplaceAllString(message, "")
	return closing_tag_regex.ReplaceAllString(message, "")
}

type inMemoryLogWriter struct{}

func (self inMemoryLogWriter) Write(p []byte) (n int, err error) {
	memory_log_mu.Lock()
	defer memory_log_mu.Unlock()

	// Truncate too many logs
	if len(memory_logs) > 1000 {
		memory_logs = nil
	}

	memory_logs = append(memory_logs, string(p))

	return len(p), nil
}
