package main

import (
	"context"

	"github.com/gavintan/gopkg/aes"
	"github.com/spf13/viper"
	"github.com/wneessen/go-mail"
)

func sendEmail(email, subject, content string) error {
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
