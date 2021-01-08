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
package common

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	gomail "gopkg.in/gomail.v2"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type MailPluginArgs struct {
	To      []string `vfilter:"required,field=to,doc=Receipient of the mail"`
	CC      []string `vfilter:"optional,field=cc,doc=A cc for the mail"`
	Subject string   `vfilter:"optional,field=subject,doc=The subject."`
	Body    string   `vfilter:"required,field=body,doc=The body of the mail."`

	// How long to wait before sending the next mail. Many mail
	// servers throttle mails sent too quickly.
	Period int64 `vfilter:"required,field=period,doc=How long to wait before sending the next mail - help to throttle mails."`
}

var (
	last_mail time.Time
)

type MailPlugin struct{}

func (self MailPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("mail: %s", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		arg := &MailPluginArgs{}
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("mail: %v", err)
			return
		}
		if time.Since(last_mail) < time.Duration(arg.Period)*time.Second {
			scope.Log("mail: Send too fast, suppressing.")
			return
		}
		last_mail = time.Now()

		if len(arg.To) == 0 {
			scope.Log("mail: no recipient.")
			return
		}

		if config_obj.Mail == nil {
			scope.Log("mail: not configured")
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

		d := gomail.NewDialer(
			config_obj.Mail.Server,
			int(port),
			config_obj.Mail.AuthUsername,
			config_obj.Mail.AuthPassword)

		// Send the email to Bob, Cora and Dan.
		err = d.DialAndSend(m)
		if err != nil {
			scope.Log("mail: %v", err)
			// Failed to send the mail but we should emit
			// the row anyway so it gets logged in the
			// artifact CSV file.
		}

		select {
		case <-ctx.Done():
			return

		case output_chan <- arg:
		}
	}()

	return output_chan
}

func (self MailPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "mail",
		Doc:     "Send Email to a remote server.",
		ArgType: type_map.AddType(scope, &MailPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&MailPlugin{})
}
