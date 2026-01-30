package mqttclient

import (
	"fmt"
	"strings"

	"github.com/techquest-tech/gin-shared/pkg/core"
)

func GetSharedTopic(topic string) string {
	appName := strings.ReplaceAll(core.AppName, " ", "_")
	return fmt.Sprintf("$share/%s/%s", appName, topic)
}
