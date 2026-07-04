package mailer

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

func TestUserWelcomeTemplateRendersUserID(t *testing.T) {
	tmpl, err := template.New("email").Option("missingkey=error").ParseFS(templateFS, "templates/user_welcome.tmpl")
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}

	data := map[string]interface{}{
		"userID":          42,
		"activationToken": "TESTTOKEN",
	}

	for _, name := range []string{"subject", "plainBody", "htmlBody"} {
		buf := new(bytes.Buffer)
		if err := tmpl.ExecuteTemplate(buf, name, data); err != nil {
			t.Fatalf("execute %q: %v", name, err)
		}
		if strings.Contains(buf.String(), "<no value>") {
			t.Errorf("%q rendered <no value>: %s", name, buf.String())
		}
	}
}
