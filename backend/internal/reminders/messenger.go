package reminders

import (
	"context"
	"log/slog"
	"strings"
)

// Messenger delivers one message to a phone number. In production this would
// be a WhatsApp Business or SMS provider (WhatsApp needs pre-approved message
// templates for messages like these). The brief asks for messaging to be
// mocked, so this build uses LogMessenger.
type Messenger interface {
	Send(ctx context.Context, phone, body string) error
}

// LogMessenger is the mock: it "sends" by writing a log line. The full text
// is also saved on the reminder row, where staff can read it in the reminder log.
type LogMessenger struct{}

// Send logs the message with the phone number masked.
func (LogMessenger) Send(_ context.Context, phone, body string) error {
	slog.Info("reminder sent (mock messenger)", "to", MaskPhone(phone), "chars", len([]rune(body)))
	return nil
}

// MaskPhone hides all but the last 4 digits, so logs do not hold phone numbers.
func MaskPhone(phone string) string {
	if len(phone) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(phone)-4) + phone[len(phone)-4:]
}
