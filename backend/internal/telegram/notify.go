package telegram

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// SendVerificationNotification posts MessageVerificationApproved to chatID
// with an inline-keyboard WebApp button pointing at the TMA. The function is
// fire-and-forget at the call site; callers wrap any error in a log instead
// of propagating to the user-facing flow (the verification side-effect has
// already committed by then).
func SendVerificationNotification(ctx context.Context, sender Sender, chatID int64, tmaURL string) error {
	params := &bot.SendMessageParams{
		ChatID: chatID,
		Text:   MessageVerificationApproved,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{
						Text:   MessageVerificationApprovedButton,
						WebApp: &models.WebAppInfo{URL: tmaURL},
					},
				},
			},
		},
	}
	_, err := sender.SendMessage(ctx, params)
	return err
}
