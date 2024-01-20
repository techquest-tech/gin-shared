package notify_test

import (
	"os"
	"testing"

	teams "github.com/atc0005/go-teams-notify/v2"

	"github.com/atc0005/go-teams-notify/v2/messagecard"
)

func TestWebhook(t *testing.T) {
	client := teams.NewTeamsClient()

	// Override the project-specific default user agent
	client.SetUserAgent("go-teams-notify-example/1.0")

	// Set webhook url.
	webhookUrl := os.Getenv("webhook_url")

	// Setup message card.
	msgCard := messagecard.NewMessageCard()
	msgCard.Title = "Hello world"
	msgCard.Text = "message 2 "
	msgCard.ThemeColor = "#DF813D"

	// Send the message with default timeout/retry settings.
	if err := client.Send(webhookUrl, msgCard); err != nil {
		panic(err)
	}

}
