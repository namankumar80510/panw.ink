// Package mail queues and sends transactional email (verification, password
// reset). There is no promotional mail in V1.
//
// Queueing inserts an email_outbox row and a Collar `send_email` job in the
// caller's transaction. The worker's job handler renders and sends the message
// without holding a database transaction across SMTP, then scrubs the
// one-time link from the row.
package mail

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/xmfunew/collar"

	"github.com/namankumar80510/panw.ink/internal/db"
)

// JobKind is the Collar job kind that sends one email_outbox row.
const JobKind = "send_email"

// Templates.
const (
	TemplateVerifyEmail      = "verify_email"
	TemplateResetPassword    = "reset_password"
	TemplateExistingAccount  = "existing_account"
	TemplatePasswordChanged  = "password_changed"
	maxSendAttempts          = 8
	smtpTimeout              = 20 * time.Second
	errorCodeSMTP            = "smtp_error"
	errorCodeTemplateMissing = "unknown_template"
)

// Enqueuer is the part of *collar.Jobs used here.
type Enqueuer interface {
	EnqueueTx(ctx context.Context, tx *sql.Tx, kind string, payload any, opts ...collar.EnqueueOptions) (uint64, error)
}

// Enqueue queues one message inside tx.
func Enqueue(ctx context.Context, tx *sql.Tx, t db.Tables, jobs Enqueuer, userID *uint64, recipient, template string, data map[string]string) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	res, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("email_outbox")+
		" (user_id, recipient, template, template_data_json) VALUES (?,?,?,?)", userID, recipient, template, string(raw))
	if err != nil {
		return fmt.Errorf("mail: outbox insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("mail: outbox id: %w", err)
	}
	_, err = jobs.EnqueueTx(ctx, tx, JobKind, id, collar.EnqueueOptions{UniqueKey: "mail:" + strconv.FormatInt(id, 10), MaxAttempts: maxSendAttempts})
	if err != nil {
		return fmt.Errorf("mail: enqueue: %w", err)
	}
	return nil
}

// Message is a rendered email.
type Message struct {
	To, Subject, Body string
}

// Sender delivers rendered messages.
type Sender interface {
	Send(ctx context.Context, from string, m Message) error
}

// SMTPSender sends through one SMTP relay (Mailpit in development).
// net/smtp upgrades with STARTTLS when the server offers it and refuses
// PLAIN auth over an unencrypted non-local connection.
type SMTPSender struct {
	Host, Username, Password string
	Port                     int
}

// Send implements Sender.
func (s SMTPSender) Send(ctx context.Context, from string, m Message) error {
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	var auth smtp.Auth
	if s.Username != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
	}
	msg := buildMIME(from, m)
	done := make(chan error, 1)
	go func() { done <- smtp.SendMail(addr, auth, from, []string{m.To}, msg) }()
	timer := time.NewTimer(smtpTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errors.New("mail: smtp timeout")
	}
}

func buildMIME(from string, m Message) []byte {
	var b strings.Builder
	b.WriteString("From: " + headerSafe(from) + "\r\n")
	b.WriteString("To: " + headerSafe(m.To) + "\r\n")
	b.WriteString("Subject: " + headerSafe(m.Subject) + "\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(strings.ReplaceAll(m.Body, "\n", "\r\n"))
	return []byte(b.String())
}

// headerSafe strips CR/LF so values cannot inject headers.
func headerSafe(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// Render builds a message from a template and its data.
func Render(appName, template, to string, data map[string]string) (Message, error) {
	switch template {
	case TemplateVerifyEmail:
		return Message{To: to, Subject: "Confirm your " + appName + " email address", Body: fmt.Sprintf(
			"Hello,\n\nPlease confirm your email address for %s by opening this link:\n\n%s\n\nThe link expires in 48 hours. If you did not create an account, you can ignore this message.\n\n— %s\n",
			appName, data["link"], appName)}, nil
	case TemplateResetPassword:
		return Message{To: to, Subject: "Reset your " + appName + " password", Body: fmt.Sprintf(
			"Hello,\n\nSomeone asked to reset the password of your %s account. To choose a new password, open this link:\n\n%s\n\nThe link expires in 1 hour and works once. If this was not you, ignore this message; your password is unchanged.\n\n— %s\n",
			appName, data["link"], appName)}, nil
	case TemplateExistingAccount:
		return Message{To: to, Subject: "Your " + appName + " account", Body: fmt.Sprintf(
			"Hello,\n\nSomeone tried to create a %s account with this email address, but an account already exists. If this was you, sign in or reset your password here:\n\n%s\n\nIf it was not you, no action is needed.\n\n— %s\n",
			appName, data["link"], appName)}, nil
	case TemplatePasswordChanged:
		return Message{To: to, Subject: "Your " + appName + " password was changed", Body: fmt.Sprintf(
			"Hello,\n\nThe password of your %s account was just changed and other sessions were signed out. If this was not you, reset your password immediately:\n\n%s\n\n— %s\n",
			appName, data["link"], appName)}, nil
	}
	return Message{}, fmt.Errorf("mail: %s: %s", errorCodeTemplateMissing, template)
}

// Handler is the worker side: it sends queued messages.
type Handler struct {
	DB      *sql.DB
	Tables  db.Tables
	Sender  Sender
	From    string
	AppName string
}

// HandleJob is the Collar job handler for JobKind.
func (h *Handler) HandleJob(ctx context.Context, j *collar.Job) error {
	var id uint64
	if err := json.Unmarshal(j.Payload, &id); err != nil {
		return fmt.Errorf("mail: payload: %w", err)
	}
	var recipient, template, raw, state string
	err := h.DB.QueryRowContext(ctx, "SELECT recipient, template, template_data_json, state FROM "+h.Tables.T("email_outbox")+" WHERE id = ?", id).
		Scan(&recipient, &template, &raw, &state)
	if errors.Is(err, sql.ErrNoRows) || err == nil && state != "queued" {
		return nil // already sent/failed, or gone: idempotent no-op
	}
	if err != nil {
		return fmt.Errorf("mail: load %d: %w", id, err)
	}
	var data map[string]string
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return h.fail(ctx, id, errorCodeTemplateMissing, true)
	}
	msg, err := Render(h.AppName, template, recipient, data)
	if err != nil {
		return h.fail(ctx, id, errorCodeTemplateMissing, true)
	}
	if err := h.Sender.Send(ctx, h.From, msg); err != nil {
		final := j.Attempts >= j.MaxAttempts
		if ferr := h.fail(ctx, id, errorCodeSMTP, final); ferr != nil {
			return ferr
		}
		// Never log the message body: it carries one-time links.
		return fmt.Errorf("mail: send outbox %d: %w", id, err)
	}
	_, err = h.DB.ExecContext(ctx, "UPDATE "+h.Tables.T("email_outbox")+
		" SET state = 'sent', sent_at = UTC_TIMESTAMP(6), attempts = attempts + 1, template_data_json = '{}', last_error_code = NULL WHERE id = ? AND state = 'queued'", id)
	if err != nil {
		return fmt.Errorf("mail: mark sent %d: %w", id, err)
	}
	return nil
}

func (h *Handler) fail(ctx context.Context, id uint64, code string, final bool) error {
	q := "UPDATE " + h.Tables.T("email_outbox") + " SET attempts = attempts + 1, last_error_code = ?, next_attempt_at = UTC_TIMESTAMP(6)"
	if final {
		q += ", state = 'failed', template_data_json = '{}'"
	}
	if _, err := h.DB.ExecContext(ctx, q+" WHERE id = ? AND state = 'queued'", code, id); err != nil {
		return fmt.Errorf("mail: record failure %d: %w", id, err)
	}
	return nil
}
