package notify_test

import (
	"testing"

	teams "github.com/atc0005/go-teams-notify/v2"

	"github.com/atc0005/go-teams-notify/v2/messagecard"
)

func TestWebhook(t *testing.T) {
	client := teams.NewTeamsClient()

	// Override the project-specific default user agent
	client.SetUserAgent("go-teams-notify-example/1.0")

	// Set webhook url.
	webhookUrl := "https://esquel.webhook.office.com/webhookb2/5df2a0e1-cd78-46e4-a28a-1d3a6ce6d77f@29abf16e-95a2-4d13-8d51-6db1b775d45b/IncomingWebhook/dd302b37f0b14a4c925b2567f3ca6a61/748e4c17-20cb-40e4-9aee-c3c16ef7f1d3"

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
