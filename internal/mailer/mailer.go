package mailer

import (
	"bytes"
	"embed"
	"html/template"
	"time"

	"github.com/wneessen/go-mail"
)

//go:embed "templates"
var templateFS embed.FS

type Mailer struct {
	host     string
	port     int
	username string
	password string
	sender   string
}

func New(host string, port int, username, password, sender string) Mailer {
	return Mailer{
		host:     host,
		port:     port,
		username: username,
		password: password,
		sender:   sender,
	}
}

func (m Mailer) Send(recipient, templateFile string, data interface{}) error {
	tmpl, err := template.New("email").ParseFS(templateFS, "templates/"+templateFile)
	if err != nil {
		return err
	}

	subject := new(bytes.Buffer)
	err = tmpl.ExecuteTemplate(subject, "subject", data)
	if err != nil {
		return err
	}

	plainBody := new(bytes.Buffer)
	err = tmpl.ExecuteTemplate(plainBody, "plainBody", data)
	if err != nil {
		return err
	}

	htmlBody := new(bytes.Buffer)
	err = tmpl.ExecuteTemplate(htmlBody, "htmlBody", data)
	if err != nil {
		return err
	}

	msg := mail.NewMsg()
	if err := msg.To(recipient); err != nil {
		return err
	}
	if err := msg.From(m.sender); err != nil {
		return err
	}
	msg.Subject(subject.String())
	msg.SetBodyString(mail.TypeTextPlain, plainBody.String())
	msg.AddAlternativeString(mail.TypeTextHTML, htmlBody.String())

	client, err := mail.NewClient(m.host,
		mail.WithPort(m.port),
		mail.WithTimeout(5*time.Second),
		mail.WithTLSPolicy(mail.TLSOpportunistic),
		mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
		mail.WithUsername(m.username),
		mail.WithPassword(m.password),
	)
	if err != nil {
		return err
	}

	for i := 1; i <= 3; i++ {
		err = client.DialAndSend(msg)
		if err == nil {
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	return err
}
