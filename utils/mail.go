package utils

import (
	mailjet "github.com/mailjet/mailjet-apiv3-go/v4"
)

func SendEmail(toEmail, subject, htmlBody string) error {
	apiKey := GetEnv("MAILJET_API_KEY", "")
	secretKey := GetEnv("MAILJET_SECRET_KEY", "")
	if apiKey == "" || secretKey == "" {
		return nil
	}

	client := mailjet.NewMailjetClient(apiKey, secretKey)
	messages := mailjet.MessagesV31{Info: []mailjet.InfoMessagesV31{
		{
			From: &mailjet.RecipientV31{
				Email: GetEnv("MAILJET_FROM_EMAIL", "noreply@yourapp.com"),
				Name:  GetEnv("MAILJET_FROM_NAME", "EnvTrack"),
			},
			To: &mailjet.RecipientsV31{
				{Email: toEmail},
			},
			Subject:  subject,
			HTMLPart: htmlBody,
		},
	}}
	_, err := client.SendMailV31(&messages)
	return err
}

func FrontendBaseURL() string {
	return GetEnv("FRONTEND_URL", "https://ecosystem-front.vercel.app")
}
