package httpclient

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/avast/retry-go"
	"go.uber.org/zap"
)

func RequestWithRetry(req *http.Request, result interface{}) error {
	log := zap.L().With(zap.String("service", "clientWithRetry"))

	client := &http.Client{}
	err := retry.Do(func() error {
		// log.Info("request to upstream", zap.String("endpoint", url))
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		cached, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		log.Info("agent replied", zap.Int("statusCode", resp.StatusCode), zap.String("status", resp.Status))
		log.Debug("resp body", zap.String("resp", string(cached)))

		err = json.Unmarshal(cached, result)
		if err != nil {
			log.Error("decode resp to object failed.", zap.Error(err))
			return err
		}
		log.Info("request upstream done.", zap.Any("result", result))
		return nil
	})
	return err
}
