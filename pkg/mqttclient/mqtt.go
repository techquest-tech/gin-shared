package mqttclient

import (
	"crypto/tls"
	"crypto/x509"
	"math"
	"os"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
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
	Cleansession  bool
	AutoReconnect bool
	Qos           byte
	TlsConfig     *TlsConfig
	subs          map[string]mqtt.MessageHandler
	Logger        *zap.Logger
	Client        mqtt.Client
}

func init() {
	core.GetContainer().Provide(InitMqtt)
}

func (m *MqttService) Sub(topic string, handle mqtt.MessageHandler) {
	if m.subs == nil {
		m.subs = make(map[string]mqtt.MessageHandler)
	}
	if _, ok := m.subs[topic]; !ok {
		m.subs[topic] = handle
	}

	token := m.Client.Subscribe(topic, m.Qos, handle)
	select {
	case <-token.Done():
		err := token.Error()
		m.Logger.Error("sub topic failed", zap.String("topic", topic), zap.Error(err))
	default:
		m.Logger.Info("sub topic done", zap.String("topic", topic))
	}

}

func (m *MqttService) OnConnect(c mqtt.Client) {
	m.Logger.Info("connected to mqtt", zap.String("endpoint", m.Endpoint))
	for topic, handle := range m.subs {
		go m.Sub(topic, handle)
	}
}

func (m *MqttService) LogMessage(c mqtt.Client, msg mqtt.Message) {
	m.Logger.Info("received message", zap.String("ClientID", m.ClientID), zap.String("topic", msg.Topic()))
	m.Logger.Info("message body", zap.String("msg", string(msg.Payload())))
}

func NewMqttOptions(logger *zap.Logger) (*mqtt.ClientOptions, *MqttService, error) {
	broke := &MqttService{
		Endpoint:      "tcp://127.0.0.1:1883",
		Logger:        logger,
		Qos:           0,
		AutoReconnect: true,
		subs:          make(map[string]mqtt.MessageHandler),
	}

	settings := viper.Sub("mqtt")
	if settings != nil {
		settings.Unmarshal(broke)
	}

	if broke.ClientID == "" {
		broke.ClientID = strings.ReplaceAll(core.AppName, " ", "_") + randstr.Hex(16)
		broke.Cleansession = true
		logger.Warn("MQTT clientID is empty, use UUID as clientID")
	}

	opts := mqtt.NewClientOptions().AddBroker(broke.Endpoint).
		SetClientID(broke.ClientID).
		SetCleanSession(broke.Cleansession).
		SetAutoReconnect(broke.AutoReconnect).
		SetMaxResumePubInFlight(math.MaxInt32)

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
				return nil, nil, err
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
				return nil, nil, err
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}

		opts.SetTLSConfig(tlsConfig)
	}

	if broke.User != "" {
		opts.Username = broke.User
		opts.Password = broke.Password
	}
	return opts, broke, nil
}

func InitMqtt(logger *zap.Logger) (*MqttService, error) {
	opts, broke, err := NewMqttOptions(logger)
	if err != nil {
		return nil, err
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
