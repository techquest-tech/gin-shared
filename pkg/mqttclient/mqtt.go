package mqttclient

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/schedule"
	"github.com/thanhpk/randstr"
	"go.uber.org/zap"
)

type TlsConfig struct {
	Ca   string
	Key  string
	Cert string
}

type MqttService struct {
	Endpoint      string
	User          string
	Password      string
	ClientID      string
	MaxPubWindow  int
	Cleansession  bool
	AutoReconnect bool
	Qos           byte
	TlsConfig     *TlsConfig
	subs          map[string]mqtt.MessageHandler
	Logger        *zap.Logger
	Client        mqtt.Client
}

// func init() {
// 	core.GetContainer().Provide(InitMqtt)
// }

var ll = sync.Mutex{}

func (m *MqttService) Sub(topic string, handle mqtt.MessageHandler) error {
	ll.Lock()
	if m.subs == nil {
		m.subs = make(map[string]mqtt.MessageHandler)
	}
	_, exists := m.subs[topic]
	if !exists {
		m.subs[topic] = handle
	}
	h := m.subs[topic]
	ll.Unlock()

	if exists {
		m.Logger.Info("topic already subscribed, skip duplicate", zap.String("topic", topic))
		return nil
	}

	token := m.Client.Subscribe(topic, m.Qos, h)
	ok := token.WaitTimeout(5 * time.Second)
	err := token.Error()
	if !ok || err != nil {
		m.Logger.Error("sub topic failed or timeout", zap.String("topic", topic), zap.Error(err))
		return err
	}
	m.Logger.Info("sub topic done", zap.String("topic", topic))
	return nil
}

func (m *MqttService) OnConnect(c mqtt.Client) {
	m.Logger.Info("connected to mqtt", zap.String("endpoint", m.Endpoint))
	ll.Lock()
	copied := make(map[string]mqtt.MessageHandler, len(m.subs))
	for t, h := range m.subs {
		copied[t] = h
	}
	ll.Unlock()
	for topic, handle := range copied {
		go func(tp string, h mqtt.MessageHandler) {
			token := m.Client.Subscribe(tp, m.Qos, h)
			ok := token.WaitTimeout(5 * time.Second)
			if !ok || token.Error() != nil {
				m.Logger.Error("resubscribe failed or timeout", zap.String("topic", tp), zap.Error(token.Error()))
				return
			}
			m.Logger.Info("resubscribe done", zap.String("topic", tp))
		}(topic, handle)
	}
}

func (m *MqttService) Pub(topic string, qos byte, retained bool, payload any) error {
	var data []byte

	switch v := payload.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		data, _ = json.Marshal(payload)
	}
	token := m.Client.Publish(topic, qos, retained, data)
	done := token.WaitTimeout(5 * time.Second)
	if !done || token.Error() != nil {
		m.Logger.Error("publish message failed or timeout", zap.String("topic", topic), zap.Error(token.Error()))
		return fmt.Errorf("pub message time out or failed %s", token.Error())
	}
	m.Logger.Info("publish  message done", zap.String("topic", topic))
	return nil
}

func (m *MqttService) LogMessage(c mqtt.Client, msg mqtt.Message) {
	m.Logger.Info("received message", zap.String("ClientID", m.ClientID), zap.String("topic", msg.Topic()))
	m.Logger.Debug("message body", zap.String("msg", string(msg.Payload())))
}

func (m *MqttService) StartHeartbeat(serviceNmae, hbSchedule string) {
	host, err := os.Hostname()
	if err != nil {
		m.Logger.Error("get hostname failed", zap.Error(err))
		host = "unknown"
	}
	topic := fmt.Sprintf("summations/healthz/%s/%s/%s/%s/heartbeat", core.AppName, core.Version, serviceNmae, host)
	topic = strings.ReplaceAll(topic, " ", "")

	schedule.CreateSchedule("heartbeat-"+serviceNmae, hbSchedule, func() {
		payload := map[string]any{
			"timestamp": time.Now(),
			"app":       core.AppName,
			"version":   core.Version,
			"module":    serviceNmae,
		}
		m.Pub(topic, 0, false, payload)
	}, schedule.ScheduleOptions{
		Nolocker:  true,
		NoHistory: true,
	})
}

func newMQTTClient(logger *zap.Logger, broke *MqttService, subKey string) (*MqttService, error) {
	if broke.subs == nil {
		broke.subs = make(map[string]mqtt.MessageHandler)
	}
	err := viper.UnmarshalKey(subKey, broke)
	if err != nil {
		return nil, err
	}

	if broke.ClientID == "" {
		broke.ClientID = strings.ReplaceAll(core.AppName, " ", "_") + randstr.Hex(16)
		logger.Warn("MQTT clientID is empty, use UUID as clientID")
	} else {
		if strings.Contains(broke.ClientID, "{{.hostname}}") {
			hs, _ := os.Hostname()
			broke.ClientID = strings.ReplaceAll(broke.ClientID, "{{.hostname}}", hs)
		}
	}

	opts := mqtt.NewClientOptions().AddBroker(broke.Endpoint).
		SetClientID(broke.ClientID).SetAutoReconnect(broke.AutoReconnect)

	if broke.Cleansession {
		opts.SetCleanSession(broke.Cleansession)
	}

	if broke.MaxPubWindow > 0 {
		opts.SetMaxResumePubInFlight(broke.MaxPubWindow)
	}

	// check if SSL enabled
	if strings.HasPrefix(broke.Endpoint, "ssl://") {
		if broke.TlsConfig == nil {
			broke.TlsConfig = &TlsConfig{
				Ca:   "config/certs/ca.pem",
				Key:  "config/certs/client.key",
				Cert: "config/certs/client.pem",
			}
		}
		logger.Info("mqtt ssl enabled")

		tlsConfig := &tls.Config{}
		if broke.TlsConfig.Ca != "" {
			// load Ca cert
			caCert, err := os.ReadFile(broke.TlsConfig.Ca)
			if err != nil {
				logger.Error("load ca cert failed", zap.Error(err))
				return nil, err
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			tlsConfig.RootCAs = caCertPool
		}

		if broke.TlsConfig.Cert != "" && broke.TlsConfig.Key != "" {

			// load client cert
			cert, err := tls.LoadX509KeyPair(broke.TlsConfig.Cert, broke.TlsConfig.Key)
			if err != nil {
				logger.Error("load client cert failed", zap.Error(err))
				return nil, err
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}

		opts.SetTLSConfig(tlsConfig)
	}

	if broke.User != "" {
		opts.Username = broke.User
		opts.Password = broke.Password
	}

	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		logger.Error("connection lost", zap.Error(err))
	}
	opts.OnConnect = broke.OnConnect

	c := mqtt.NewClient(opts)

	if token := c.Connect(); token.Wait() && token.Error() != nil {
		logger.Error("connect to mqtt failed.", zap.String("endpoint", broke.Endpoint), zap.Error(token.Error()))
		return nil, token.Error()
	}

	broke.Client = c

	logger.Info("mqtt init done.", zap.String("endpoint", broke.Endpoint),
		zap.String("clientID", broke.ClientID), zap.Int("qos", int(broke.Qos)),
		zap.Bool("cleansession", broke.Cleansession))

	core.OnServiceStopping(func() {
		c.Disconnect(1000)
		logger.Info("mqtt client stopped")
	})
	return broke, nil
}

func initMqtt(logger *zap.Logger) (*MqttService, error) {
	return newMQTTClient(logger, &MqttService{
		Endpoint: "tcp://127.0.0.1:1883",
		Logger:   logger,
		Qos:      1,
		// Cleansession:  true,
		AutoReconnect: true,
	}, "mqtt")
}

func InitMqttService(clientID string, qos byte, cleansession bool, yamlKey string) (*MqttService, error) {
	if yamlKey == "" {
		yamlKey = "mqtt"
	}
	return newMQTTClient(zap.L(), &MqttService{
		ClientID:      clientID,
		Endpoint:      "tcp://127.0.0.1:1883",
		Logger:        zap.L(),
		Qos:           qos,
		Cleansession:  cleansession,
		AutoReconnect: true,
	}, yamlKey)
}

func init() {
	core.Provide(initMqtt)
}
