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
package networking

import (
	"context"
	"errors"
	"time"

	"github.com/Velocidex/ordereddict"
	gomail "gopkg.in/gomail.v2"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type MailPluginArgs struct {
	To      []string `vfilter:"required,field=to,doc=Recipient of the mail"`
	From    string   `vfilter:"optional,field=from,doc=The from email address."`
	CC      []string `vfilter:"optional,field=cc,doc=A cc for the mail"`
	Subject string   `vfilter:"optional,field=subject,doc=The subject."`
	Body    string   `vfilter:"required,field=body,doc=The body of the mail."`

	// How long to wait before sending the next mail. Many mail
	// servers throttle mails sent too quickly.
	Period int64 `vfilter:"optional,field=period,doc=How long to wait before sending the next mail - help to throttle mails."`

	ServerPort   uint64            `vfilter:"optional,field=server_port,doc=The SMTP server port to use (default 587)."`
	Server       string            `vfilter:"optional,field=server,doc=The SMTP server to use (if not specified we try the config file)."`
	AuthUsername string            `vfilter:"optional,field=auth_username,doc=The SMTP username we authenticate to the server."`
	AuthPassword string            `vfilter:"optional,field=auth_password,doc=The SMTP username password we use to authenticate to the server."`
	SkipVerify   bool              `vfilter:"optional,field=skip_verify,doc=Skip SSL verification(default: False)."`
	RootCerts    string            `vfilter:"optional,field=root_ca,doc=As a better alternative to disable_ssl_security, allows root ca certs to be added here."`
	Secret       string            `vfilter:"optional,field=secret,doc=Alternatively use a secret from the secrets service. Secret must be of type 'SMTP Creds'"`
	Headers      *ordereddict.Dict `vfilter:"optional,field=headers,doc=A dict of headers to send."`
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

		select {
		case <-ctx.Done():
			return

		case output_chan <- MailFunction{}.Call(ctx, scope, args):
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

type MailFunction struct{}

func (self MailFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "mail", args)()

	res := ordereddict.NewDict()

	err := vql_subsystem.CheckAccess(scope, acls.NETWORK)
	if err != nil {
		scope.Log("ERROR:mail: %s", err)
		return res.Set("ErrorStatus", err.Error())
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("ERROR:mail: Command can only run on the server")
		return res.Set("ErrorStatus", err.Error())
	}

	arg := &MailPluginArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("ERROR:mail: %v", err)
		return res.Set("ErrorStatus", err.Error())
	}

	err = self.maybeForceSecrets(ctx, scope, arg)
	if err != nil {
		scope.Log("ERROR:mail: %v", err)
		return res.Set("ErrorStatus", err.Error())
	}

	if arg.Secret != "" {
		err := self.mergeSecretToRequest(ctx, scope, arg, arg.Secret)
		if err != nil {
			scope.Log("ERROR:mail: %v", err)
			return res.Set("ErrorStatus", err.Error())
		}
	}

	if arg.Period == 0 {
		arg.Period = 60
	}
	if time.Since(last_mail) < time.Duration(arg.Period)*time.Second {
		scope.Log("ERROR:mail: Send too fast, suppressing.")
		return res.Set("ErrorStatus", "Send too fast, suppressing.")
	}
	last_mail = time.Now()

	if len(arg.To) == 0 {
		scope.Log("ERROR:mail: no recipient.")
		return res.Set("ErrorStatus", "no recipient.")
	}

	// Allow global configuration override.
	mail_config := config_obj.Mail
	if mail_config == nil {
		mail_config = &config_proto.MailConfig{}
	}

	auth_username := arg.AuthUsername
	if auth_username == "" {
		auth_username = mail_config.AuthUsername
	}

	auth_password := arg.AuthPassword
	if auth_password == "" {
		auth_password = mail_config.AuthPassword
	}

	from := arg.From
	if from == "" {
		from = mail_config.From
	}

	if from == "" {
		from = auth_username
	}

	if from == "" {
		from = "no-reply@velociraptor.example.com"
	}

	m := gomail.NewMessage(gomail.SetEncoding(gomail.Unencoded))
	m.SetHeader("From", from)
	m.SetHeader("To", arg.To...)
	if len(arg.CC) > 0 {
		m.SetHeader("Cc", arg.CC...)
	}
	m.SetHeader("Subject", arg.Subject)
	content_type := "text/plain"

	if arg.Headers != nil {
		for _, k := range arg.Headers.Keys() {
			v, ok := arg.Headers.GetString(k)
			if ok {
				m.SetHeader(k, v)
			}
			if k == "Content-Type" {
				content_type = v
			}
		}
	}
	m.SetBody(content_type, arg.Body)

	port := arg.ServerPort
	if port == 0 {
		port = mail_config.ServerPort
	}
	if port == 0 {
		port = 587 // Default to TLS SMTP
	}

	server := arg.Server
	if server == "" {
		server = mail_config.Server
	}
	if server == "" {
		scope.Log("ERROR:mail: server not specified")
		return res.Set("ErrorStatus", "server not specified")
	}

	d := gomail.NewDialer(server, int(port), auth_username, auth_password)

	// Skip verification of the TLS connection
	if arg.SkipVerify || mail_config.SkipVerify {
		d.TLSConfig, err = GetSkipVerifyTlsConfig(config_obj.GetClient())
		if err != nil {
			scope.Log("mail: could not get SkipVerify enabled TLS config: %s", err)
			return res.Set("ErrorStatus", err.Error())
		}

	} else if config_obj.Client != nil {
		// Try to use our standard methods for getting TLS config up
		d.TLSConfig, err = GetTlsConfig(config_obj.Client, arg.RootCerts)
		if err != nil {
			scope.Log("mail: could not get TLS config: %s", err)
			return res.Set("ErrorStatus", err.Error())
		}
	}

	err = d.DialAndSend(m)
	if err != nil {
		scope.Log("ERROR:mail: %v", err)
		// Failed to send the mail but we should emit
		// the row anyway so it gets logged in the
		// artifact CSV file.
		return res.Set("ErrorStatus", err.Error())
	}

	return res.Set("ErrorStatus", "OK: Message Sent")
}

func (self *MailFunction) maybeForceSecrets(
	ctx context.Context, scope vfilter.Scope,
	arg *MailPluginArgs) error {

	// Not running on the server, secrets dont work.
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return nil
	}

	if config_obj.Security == nil {
		return nil
	}

	if !config_obj.Security.VqlMustUseSecrets {
		return nil
	}

	// If an explicit secret is defined let it filter the URLs.
	if arg.Secret != "" {
		return nil
	}

	return utils.SecretsEnforced
}

func (self MailFunction) mergeSecretToRequest(
	ctx context.Context, scope vfilter.Scope,
	arg *MailPluginArgs, secret_name string) error {

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return errors.New("Secrets may only be used on the server")
	}

	secrets_service, err := services.GetSecretsService(config_obj)
	if err != nil {
		return err
	}

	principal := vql_subsystem.GetPrincipal(scope)

	secret_record, err := secrets_service.GetSecret(ctx, principal,
		constants.SMTP_CREDS, secret_name)
	if err != nil {
		return err
	}

	// Replace the following args from the secret - do not allow users
	// to override them.
	arg.Server = secret_record.GetString("server")
	arg.ServerPort = secret_record.GetUint64("server_port")
	arg.AuthUsername = secret_record.GetString("auth_username")
	arg.AuthPassword = secret_record.GetString("auth_password")
	arg.RootCerts = secret_record.GetString("root_ca")
	arg.SkipVerify = secret_record.GetBool("skip_verify")

	// Optional parameters may be set in the secret.
	secret_record.UpdateString("from", &arg.From)

	return nil
}

func (self MailFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "mail",
		Doc:      "Send Email to a remote server.",
		ArgType:  type_map.AddType(scope, &MailPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.NETWORK).Build(),
	}
}

func init() {
	// This is the old style plugin but really mail() should be a
	// function since it can only ever return one row.
	vql_subsystem.RegisterPlugin(&MailPlugin{})
	vql_subsystem.RegisterFunction(&MailFunction{})
}
