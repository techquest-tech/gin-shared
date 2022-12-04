package mqttclient

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

type MqttService struct {
	Endpoint     string
	User         string
	Password     string
	ClientID     string
	Cleansession bool
	Qos          byte
	subs         map[string]mqtt.MessageHandler
	Logger       *zap.Logger
	Client       mqtt.Client
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

	m.Logger.Info("received message", zap.String("ID", m.ClientID), zap.String("topic", msg.Topic()))
	m.Logger.Info("message body", zap.String("msg", string(msg.Payload())))
}

func InitMqtt(logger *zap.Logger) (*MqttService, error) {
	broke := &MqttService{
		Endpoint: "tcp://127.0.0.1:1883",
		Logger:   logger,
		ClientID: core.AppName,
		subs:     make(map[string]mqtt.MessageHandler),
	}

	settings := viper.Sub("mqtt")
	if settings != nil {
		settings.Unmarshal(broke)
	}

	opts := mqtt.NewClientOptions().AddBroker(broke.Endpoint).
		SetClientID(broke.ClientID).
		SetCleanSession(broke.Cleansession)

	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		logger.Error("connection lost", zap.Error(err))
	}
	opts = opts.SetAutoReconnect(true)

	if broke.User != "" {
		opts.Username = broke.User
		opts.Password = broke.Password
	}
	opts.OnConnect = broke.OnConnect

	c := mqtt.NewClient(opts)

	if token := c.Connect(); token.Wait() && token.Error() != nil {
		logger.Error("connect to mqtt failed.", zap.String("endpoint", broke.Endpoint), zap.Error(token.Error()))
		return nil, token.Error()
	}

	broke.Client = c

	logger.Info("mqtt init done.", zap.String("endpoint", broke.Endpoint), zap.Bool("cleansession", broke.Cleansession))
	return broke, nil
}
