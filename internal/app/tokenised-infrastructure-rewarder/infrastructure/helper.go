package infrastructure

import (
	"fmt"
	"time"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

func NewHelper(config *Config) *Helper {
	return &Helper{config: *config}
}

type Helper struct {
	config Config
}

// We generate a date one month on from the target one (m+1), but set the day of month to 0.
// Days are 1-indexed, so this has the effect of rolling back one day to the last day of the previous month (our target month of m).
// Calling Day() then procures the number we want.
// Returns the length of the month
func (h *Helper) DaysIn(m time.Month, year int) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func (h *Helper) Unix() int64 {
	return time.Now().Unix()
}

func (h *Helper) Date() (year int, month time.Month, day int) {
	return time.Now().Date()
}

func (h *Helper) SendMail(message string) error {
	from := mail.NewEmail("Aura Pay Service", h.config.MailFromAddress)
	subject := "Automatic email from Aura Pay Service"
	to := mail.NewEmail("User", h.config.MailToAddress)
	plainTextContent := message
	htmlContent := fmt.Sprintf("<strong>%s</strong>", message)
	emailMsg := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)
	client := sendgrid.NewSendClient(h.config.SendgridApiKey)
	_, err := client.Send(emailMsg)
	if err != nil {
		return err
	}
	return nil
}
