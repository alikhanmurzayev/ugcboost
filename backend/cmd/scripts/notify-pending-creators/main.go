// notify-pending-creators is a one-shot local script that re-sends the
// Instagram-verify welcome to creators whose applications were linked
// before that step was added to the bot flow. Reads a hard-coded chat_id
// -> verification_code map and pushes the same text current new
// applications receive on /start.
//
// Usage:
//
//	cd backend
//	TELEGRAM_BOT_TOKEN=xxx go run ./cmd/scripts/notify-pending-creators
//	TELEGRAM_BOT_TOKEN=xxx go run ./cmd/scripts/notify-pending-creators -dry-run
//
// Uses telegram.NewSendOnlyBot — does not start getUpdates polling, so
// the prod bot remains the sole consumer of incoming updates.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"html"
	"os"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
)

// recipients maps Telegram chat_id -> verification code (format UGC-NNNNNN).
// Populate from the prod query before running. Empty map aborts the script.
var recipients = map[int64]string{}

// welcomeWithIGTemplate mirrors internal/telegram/notifier.go. Duplicated
// here so a disposable backfill does not widen the package surface. If the
// prod template changes before this script is removed, sync it manually.
const welcomeWithIGTemplate = "Здравствуйте! 👋\n\n" +
	"Мы получили вашу заявку.\n" +
	"Подтвердите, пожалуйста, что вы действительно владеете указанным аккаунтом Instagram:\n\n" +
	"1. Скопируйте код:\n" +
	"   <pre>%s</pre>\n\n" +
	"2. Откройте Direct и отправьте его нам:\n\n" +
	"   https://ig.me/m/ugc_boost"

// sendDelay paces the broadcast at ~10 msg/sec — well under Telegram's
// 30 msg/sec per-bot ceiling for messages to different chats.
const sendDelay = 100 * time.Millisecond

func main() {
	dryRun := flag.Bool("dry-run", false, "print recipients without sending")
	flag.Parse()

	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		fmt.Fprintln(os.Stderr, "TELEGRAM_BOT_TOKEN is required")
		os.Exit(1)
	}
	if len(recipients) == 0 {
		fmt.Fprintln(os.Stderr, "recipients map is empty — populate it before running")
		os.Exit(1)
	}

	fmt.Printf("Recipients: %d\n", len(recipients))
	if *dryRun {
		for chatID, code := range recipients {
			fmt.Printf("  chat_id=%d code=%s\n", chatID, code)
		}
		return
	}

	if !confirm(fmt.Sprintf("Send notification to %d chats? [y/N]: ", len(recipients))) {
		fmt.Println("aborted")
		return
	}

	b, err := telegram.NewSendOnlyBot(token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create bot: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	var sent, failed int
	for chatID, code := range recipients {
		if strings.TrimSpace(code) == "" {
			failed++
			fmt.Printf("SKIP chat_id=%d empty code\n", chatID)
			continue
		}
		text := fmt.Sprintf(welcomeWithIGTemplate, html.EscapeString(code))
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      text,
			ParseMode: models.ParseModeHTML,
		})
		if sendErr != nil {
			failed++
			fmt.Printf("FAIL chat_id=%d code=%s err=%v\n", chatID, code, sendErr)
		} else {
			sent++
			fmt.Printf("OK   chat_id=%d code=%s\n", chatID, code)
		}
		time.Sleep(sendDelay)
	}

	fmt.Printf("\nDone. sent=%d failed=%d\n", sent, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	s := bufio.NewScanner(os.Stdin)
	if !s.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(s.Text()))
	return answer == "y" || answer == "yes"
}
