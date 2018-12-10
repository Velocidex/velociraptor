package common

import (
	"context"
	"time"

	gomail "gopkg.in/gomail.v2"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type MailPluginArgs struct {
	To      []string `vfilter:"required,field=to"`
	CC      []string `vfilter:"optional,field=cc"`
	Subject string   `vfilter:"optional,field=subject"`
	Body    string   `vfilter:"required,field=body"`

	// How long to wait before sending the next mail. Many mail
	// servers throttle mails sent too quickly.
	Period int64 `vfilter:"required,field=period"`
}

var (
	last_mail time.Time
)

type MailPlugin struct{}

func (self MailPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		any_config_obj, _ := scope.Resolve("server_config")
		config_obj, ok := any_config_obj.(*api_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		arg := &MailPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("mail: %v", err)
			return
		}
		if time.Since(last_mail) < time.Duration(arg.Period)*time.Second {
			scope.Log("mail: Send too fast, suppressing.")
			return
		}

		if len(arg.To) == 0 {
			scope.Log("mail: no recipient.")
			return
		}

		from := config_obj.Mail.From
		if from == "" {
			from = config_obj.Mail.AuthUsername
		}

		m := gomail.NewMessage()
		m.SetHeader("From", from)
		m.SetHeader("To", arg.To...)
		if len(arg.CC) > 0 {
			m.SetHeader("Cc", arg.CC...)
		}
		m.SetHeader("Subject", arg.Subject)
		m.SetBody("text/plain", arg.Body)

		port := config_obj.Mail.ServerPort
		if port == 0 {
			port = 587
		}

		d := gomail.NewPlainDialer(
			config_obj.Mail.Server,
			int(port),
			config_obj.Mail.AuthUsername,
			config_obj.Mail.AuthPassword)

		// Send the email to Bob, Cora and Dan.
		err = d.DialAndSend(m)
		if err != nil {
			scope.Log("mail: %v", err)
			return
		}

		output_chan <- arg
	}()

	return output_chan
}

func (self MailPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "mail",
		Doc:     "Send Email to a remote server.",
		ArgType: "MailPluginArgs",
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&MailPlugin{})
}
