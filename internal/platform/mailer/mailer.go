// Package mailer envoie les emails transactionnels.
package mailer

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// Mailer envoie un email texte.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Noop n'envoie rien : le lien d'invitation reste retourné à l'administrateur.
type Noop struct{}

// Send implémente Mailer.
func (Noop) Send(context.Context, string, string, string) error { return nil }

// SMTP envoie via un relais SMTP (Mailpit en local).
type SMTP struct {
	Addr string
	From string
}

// Send implémente Mailer.
func (m SMTP) Send(_ context.Context, to, subject, body string) error {
	if strings.ContainsAny(to, "\r\n") || strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("en-tête d'email invalide")
	}
	msg := "From: " + m.From + "\r\nTo: " + to + "\r\nSubject: " + subject +
		"\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body
	return smtp.SendMail(m.Addr, nil, m.From, []string{to}, []byte(msg))
}
