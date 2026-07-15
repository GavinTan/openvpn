package main

import (
	"context"
	"io"

	"github.com/gavintan/gopkg/aes"
	"github.com/spf13/viper"
	"github.com/wneessen/go-mail"
)

// EmailAttachment 表示一封邮件的单个附件。
// Filename 是客户端展示的文件名，Reader 是数据源（不会立即读完，只在 DialAndSend 时按需读）。
type EmailAttachment struct {
	Filename string
	Reader   io.Reader
}

func sendEmail(email, subject, content string, attachments ...EmailAttachment) error {
	sendFrom := viper.GetString("system.email.send_from")
	sendSubjectPrefix := viper.GetString("system.email.send_subject_prefix")
	host := viper.GetString("system.email.host")
	port := viper.GetInt("system.email.port")
	username := viper.GetString("system.email.username")
	password := viper.GetString("system.email.password")
	security := viper.GetString("system.email.security")

	subject = sendSubjectPrefix + subject
	password, _ = aes.AesDecrypt(password, secretKey)

	if sendFrom == "" {
		sendFrom = username
	}

	message := mail.NewMsg()
	if err := message.From(sendFrom); err != nil {
		logger.Error(context.Background(), "failed to set From address: %s", err)
		return err
	}

	if err := message.To(email); err != nil {
		logger.Error(context.Background(), "failed to set To address: %s", err)
		return err
	}

	message.Subject(subject)
	message.SetBodyString(mail.TypeTextHTML, content)

	// 添加附件（如有）。variadic 接受零值所以老调用点零改动。
	for _, a := range attachments {
		if a.Filename == "" || a.Reader == nil {
			continue
		}
		if err := message.AttachReader(a.Filename, a.Reader); err != nil {
			logger.Error(context.Background(), "failed to attach "+a.Filename+": "+err.Error())
			return err
		}
	}

	client, err := mail.NewClient(host, mail.WithPort(port), mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
		mail.WithUsername(username), mail.WithPassword(password))
	if err != nil {
		logger.Error(context.Background(), "failed to create mail client: %s", err)
		return err
	}

	switch security {
	case "tls":
		client.SetTLSPolicy(mail.TLSMandatory)
	case "ssl":
		client.SetSSL(true)
	}

	if err := client.DialAndSend(message); err != nil {
		logger.Error(context.Background(), "failed to send mail: %s", err)
		return err
	}

	return nil
}
