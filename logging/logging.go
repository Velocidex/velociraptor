package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

var (
	SuppressLogging = false

	GenericComponent  = "Velociraptor"
	FrontendComponent = "VelociraptorFrontend"
	ClientComponent   = "VelociraptorClient"
	GUIComponent      = "VelociraptorGUI"
	ToolComponent     = "Velociraptor"

	// Used for high value audit related events.
	Audit = "VelociraptorAudit"

	Manager *LogManager
)

type LogContext struct {
	*logrus.Logger
}

func (self *LogContext) Info(format string, v ...interface{}) {
	self.Logger.Info(fmt.Sprintf(format, v...))
}

type LogManager struct {
	mu       sync.Mutex
	contexts map[*string]*LogContext
}

// Get the logger from cache - creating it if it needs to.
func (self *LogManager) GetLogger(
	config_obj *api_proto.Config,
	component *string) *LogContext {
	self.mu.Lock()
	defer self.mu.Unlock()

	if !config_obj.Logging.SeparateLogsPerComponent {
		component = &GenericComponent
	}

	ctx, pres := self.contexts[component]
	if !pres {
		// Add a new context.
		switch component {
		case &GenericComponent,
			&FrontendComponent, &ToolComponent, &Audit,
			&ClientComponent, &GUIComponent:

			logger := self.makeNewComponent(config_obj, component)
			if config_obj.Logging.SeparateLogsPerComponent {
				self.contexts[component] = logger
				return logger
			} else {
				self.contexts[&GenericComponent] = logger
				return logger
			}

		default:
			panic("Unsupported component!")
		}
	}
	return ctx
}

func getRotator(base_path string) *rotatelogs.RotateLogs {
	result, err := rotatelogs.New(
		base_path+".%Y%m%d%H%M",
		rotatelogs.WithLinkName(base_path),
		// 1 day.
		rotatelogs.WithMaxAge(time.Duration(86400)*time.Second),
		// 7 days.
		rotatelogs.WithRotationTime(time.Duration(604800)*time.Second),
	)

	if err != nil {
		panic(err)
	}

	return result
}

func (self *LogManager) makeNewComponent(
	config_obj *api_proto.Config,
	component *string) *LogContext {

	Log := logrus.New()
	Log.Out = ioutil.Discard
	Log.Level = logrus.DebugLevel

	if config_obj.Logging.OutputDirectory != "" {
		base_filename := filepath.Join(
			config_obj.Logging.OutputDirectory,
			*component)

		pathMap := lfshook.WriterMap{
			logrus.DebugLevel: getRotator(base_filename + "_debug.log"),
			logrus.InfoLevel:  getRotator(base_filename + "_info.log"),
			logrus.ErrorLevel: getRotator(base_filename + "_error.log"),
		}

		hook := lfshook.NewHook(
			pathMap,
			&logrus.JSONFormatter{},
		)
		Log.Hooks.Add(hook)
	}

	stderr_map := lfshook.WriterMap{
		logrus.ErrorLevel: os.Stderr,
	}

	if !SuppressLogging {
		stderr_map[logrus.DebugLevel] = os.Stderr
		stderr_map[logrus.InfoLevel] = os.Stderr
	}

	Log.Hooks.Add(lfshook.NewHook(stderr_map, &Formatter{}))

	return &LogContext{Log}
}

type Formatter struct{}

func (self *Formatter) Format(entry *logrus.Entry) ([]byte, error) {
	b := &bytes.Buffer{}

	levelText := strings.ToUpper(entry.Level.String())
	fmt.Fprintf(b, "[%s] %v %s ", levelText, entry.Time.Format(time.RFC3339),
		entry.Message)

	if len(entry.Data) > 0 {
		serialized, _ := json.Marshal(entry.Data)
		fmt.Fprintf(b, "%s", serialized)
	}

	return append(b.Bytes(), '\n'), nil
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
	config *api_proto.Config,
	component *string) *log.Logger {
	if !SuppressLogging {
		return log.New(&logWriter{GetLogger(config, component)}, "", log.Lshortfile)
	}

	return log.New(ioutil.Discard, "", log.Lshortfile)
}

func GetLogger(config_obj *api_proto.Config, component *string) *LogContext {
	return Manager.GetLogger(config_obj, component)
}

func init() {
	Manager = &LogManager{
		contexts: make(map[*string]*LogContext),
	}
}
