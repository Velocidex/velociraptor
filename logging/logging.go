/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	rotatelogs "github.com/Velocidex/file-rotatelogs"
	"github.com/go-errors/errors"
	"github.com/mattn/go-isatty"

	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
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
	mu      sync.Mutex
	manager *LogManager

	disable_log_to_files bool
	node_name            = ""

	// Lock for memory logs and prelogs.
	memory_log_mu sync.Mutex
	prelogs       []string
	memory_logs   []string

	tag_regex         = regexp.MustCompile("<([^>/0]+)>")
	closing_tag_regex = regexp.MustCompile("</>")
)

func Manager() *LogManager {
	mu.Lock()
	defer mu.Unlock()

	return manager
}

func SetNodeName(name string) {
	mu.Lock()
	defer mu.Unlock()

	node_name = name
}

// Turn off logging to files from now on. This is needed for commands
// that manipulate the config file and we dont want to attempt to
// write to random log files.
func DisableLogging() {
	mu.Lock()
	disable_log_to_files = true
	mu.Unlock()
}

func InitLogging(config_obj *config_proto.Config) error {
	new_manager := &LogManager{
		contexts: make(map[*string]*LogContext),
	}

	components := []*string{
		&GenericComponent, &FrontendComponent, &ClientComponent,
		&GUIComponent, &ToolComponent, &APICmponent, &Audit}

	// User asked for all components to go in the same log.
	if config_obj.Logging != nil &&
		!config_obj.Logging.SeparateLogsPerComponent {
		components = []*string{&GenericComponent}
	}

	for _, component := range components {
		logger, err := new_manager.makeNewComponent(config_obj, component)
		if err != nil {
			return err
		}
		new_manager.contexts[component] = logger
	}

	err := maybeAddRemoteSyslog(context.Background(), config_obj, new_manager)
	if err != nil {
		return err
	}

	mu.Lock()
	manager = new_manager
	mu.Unlock()

	FlushPrelogs(config_obj)

	return nil
}

func ClearMemoryLogs() {
	memory_log_mu.Lock()
	memory_logs = nil
	memory_log_mu.Unlock()
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
		logger.Info("%s", msg)
	}
	prelogs = make([]string, 0)
}

type LogContext struct {
	*logrus.Logger

	mu      sync.Mutex
	enabled map[string]bool

	listeners map[uint64]chan string
	component string
}

func (self *LogContext) AddListener(c chan string) func() {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.listeners == nil {
		self.listeners = make(map[uint64]chan string, 10)
	}

	id := utils.GetId()
	self.listeners[id] = c

	return func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		delete(self.listeners, id)
	}
}

func (self *LogContext) forwardMessage(level, msg string) {
	// Avoid deadlocks by taking a copy
	self.mu.Lock()
	var listeners []chan string
	for _, c := range self.listeners {
		listeners = append(listeners, c)
	}
	self.mu.Unlock()

	for _, c := range listeners {
		msg = strings.TrimSpace(msg)

		line := json.Format(`{"time":%q,"level":%q,"msg":%q}`,
			utils.GetTime().Now().UTC().Format(time.RFC3339), level, msg)

		select {
		case c <- line:
		default:
		}
	}
}

func (self *LogContext) Debug(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	if self.Logger != nil {
		self.Logger.Debug(msg)
	}
	self.forwardMessage(DEBUG, msg)
}

func (self *LogContext) Info(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	if self.Logger != nil {
		self.Logger.Info(msg)
	}
	self.forwardMessage(INFO, msg)
}

func (self *LogContext) Warn(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	if self.Logger != nil {
		self.Logger.Warn(msg)
	}
	self.forwardMessage(WARN, msg)
}

func (self *LogContext) Error(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	if self.Logger != nil {
		self.Logger.Error(msg)
	}
	self.forwardMessage(ERROR, msg)
}

func (self *LogContext) IsEnabled(level string) bool {
	self.mu.Lock()
	defer self.mu.Unlock()
	ok, _ := self.enabled[level]
	return ok
}

func (self *LogContext) LogWithLevel(level string, format string, v ...interface{}) {
	switch level {
	case ERROR:
		self.Error(format, v...)
	case WARNING:
		self.Warn(format, v...)
	case INFO:
		self.Info(format, v...)
	case DEBUG:
		self.Debug(format, v...)
	default:
		self.Info(format, v...)
	}
}

type LogManager struct {
	mu       sync.Mutex
	contexts map[*string]*LogContext
}

func (self *LogManager) AddHook(hook logrus.Hook, component *string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	v, pres := self.contexts[component]
	if pres {
		v.Logger.Hooks.Add(hook)
	}
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
		return &LogContext{
			Logger: &logrus.Logger{
				Out:       os.Stderr,
				Formatter: new(logrus.TextFormatter),
				Hooks:     make(logrus.LevelHooks),
				Level:     logrus.DebugLevel,
			},
			component: *component,
			enabled:   make(map[string]bool),
		}
	}
	return ctx
}

func (self *LogManager) Reset() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.contexts = make(map[*string]*LogContext)
}

func Reset() {
	mu.Lock()
	defer mu.Unlock()

	if manager != nil {
		manager.Reset()
	}
}

func getRotator(
	config_obj *config_proto.Config,
	rotator_config *config_proto.LoggingRetentionConfig,
	base_path string) (io.Writer, error, bool) {

	if rotator_config == nil {
		rotator_config = &config_proto.LoggingRetentionConfig{
			RotationTime: config_obj.Logging.RotationTime,
			MaxAge:       config_obj.Logging.MaxAge,
		}
	}

	if rotator_config.Disabled {
		return ioutil.Discard, nil, false
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
		return nil, err, false
	}

	// Make sure to write one message to confirm that we can actually
	// write to the file.
	now := utils.GetTime().Now().UTC()
	_, err = result.Write([]byte(json.Format(
		"{\"level\": \"info\", \"msg\": \"Starting...\", \"time\": %q}\n", now)))
	return result, err, true
}

func (self *LogManager) makeNewComponent(
	config_obj *config_proto.Config,
	component *string) (*LogContext, error) {

	enabled := make(map[string]bool)

	Log := logrus.New()
	Log.Out = newInMemoryLogWriter()
	Log.Level = logrus.DebugLevel
	Log.Formatter = &logrus.JSONFormatter{
		DisableHTMLEscape: true,
	}

	if !disable_log_to_files &&
		config_obj != nil &&
		config_obj.Logging != nil &&
		config_obj.Logging.OutputDirectory != "" {

		output_directory := utils.ExpandEnv(config_obj.Logging.OutputDirectory)
		base_directory := filepath.Join(output_directory, node_name)
		err := os.MkdirAll(base_directory, 0700)
		if err != nil {
			return nil, errors.New("Unable to create logging directory.")
		}

		base_filename := filepath.Join(base_directory, *component)
		pathMap := lfshook.WriterMap{}

		Prelog("Initializing logging for %v\n", base_filename)

		rotator, err, enable := getRotator(
			config_obj, config_obj.Logging.Debug,
			base_filename+"_debug.log")
		if err != nil {
			return nil, err
		}
		pathMap[logrus.DebugLevel] = rotator
		enabled[DEBUG] = enable

		rotator, err, enable = getRotator(
			config_obj, config_obj.Logging.Info,
			base_filename+"_info.log")
		if err != nil {
			return nil, err
		}
		pathMap[logrus.InfoLevel] = rotator
		enabled[INFO] = enable

		rotator, err, enable = getRotator(
			config_obj, config_obj.Logging.Error,
			base_filename+"_error.log")
		if err != nil {
			return nil, err
		}
		pathMap[logrus.ErrorLevel] = rotator
		enabled[ERROR] = enable

		hook := lfshook.NewHook(
			pathMap,
			&JSONFormatter{&logrus.JSONFormatter{
				DisableHTMLEscape: true,
			}},
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

	return &LogContext{
		Logger:    Log,
		enabled:   enabled,
		component: *component,
	}, nil
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

	for _, log := range Manager().contexts {
		log.Hooks.Add(lfshook.NewHook(
			writer_map, &JSONFormatter{&logrus.JSONFormatter{
				DisableHTMLEscape: true,
			}},
		))
	}
	return nil
}

func SplitIntoLevelAndLog(b []byte) (level, message string) {
	parts := strings.SplitN(string(b), ":", 2)
	if len(parts) == 2 {
		level := strings.ToUpper(parts[0])
		switch level {
		case DEFAULT, ERROR, INFO, WARNING, DEBUG, ALERT:
			return level, parts[1]
		}
	}

	return DEFAULT, string(b)
}

type logWriter struct {
	logger *LogContext
}

func (self *logWriter) Write(b []byte) (int, error) {
	level, msg := SplitIntoLevelAndLog(b)
	self.logger.LogWithLevel(level, "%v", msg)
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
	lManager := Manager()
	if lManager == nil {
		err := InitLogging(config_obj)
		if err != nil {
			panic(err)
		}
		lManager = Manager()

	}
	return lManager.GetLogger(config_obj, component)
}

type stackTracer interface {
	Stack() []byte
}

func GetStackTrace(err error) string {
	if serr, ok := err.(stackTracer); ok {
		return string(serr.Stack())
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

func newInMemoryLogWriter() *inMemoryLogWriter {
	return &inMemoryLogWriter{}
}
