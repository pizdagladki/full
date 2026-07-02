package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

type telegramNotifier struct {
	botToken string
	chatID   string
	logger   *zap.Logger
}

// NewTelegramNotifier constructs a TelegramNotifier that posts messages via
// the Telegram Bot API using stdlib net/http.
func NewTelegramNotifier(botToken, chatID string, logger *zap.Logger) TelegramNotifier {
	return &telegramNotifier{
		botToken: botToken,
		chatID:   chatID,
		logger:   logger,
	}
}

// Notify sends a formatted bug-report message to the configured Telegram chat.
func (n *telegramNotifier) Notify(ctx context.Context, userID int64, device, description, objectRef string) error {
	text := fmt.Sprintf(
		"Bug report\nUser: %d\nDevice: %s\nDescription: %s",
		userID, device, description,
	)
	if objectRef != "" {
		text += "\nRecording: " + objectRef
	}

	payload := struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}{
		ChatID: n.chatID,
		Text:   text,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram payload: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.botToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create telegram request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send telegram message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	return nil
}
