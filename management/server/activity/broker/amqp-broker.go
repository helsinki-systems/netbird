package broker

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/netbirdio/netbird/management/server/activity"

	amqp "github.com/rabbitmq/rabbitmq-amqp-go-client/pkg/rabbitmqamqp"
)

const (
	amqpBrokerUrlEnv   = "NB_ACTIVITY_EVENT_BROKER_URL"
	amqpBrokerExchange = "NB_ACTIVITY_EVENT_BROKER_EXCHANGE"
)

type Broker struct {
	environment *amqp.Environment
	publisher   *amqp.Publisher
}

func SetupClient(ctx context.Context) (*Broker, error) {
	brokerUrl, ok := os.LookupEnv(amqpBrokerUrlEnv)
	if !ok {
		return nil, fmt.Errorf("%s environment variable not set", amqpBrokerUrlEnv)
	}
	endpoints := []amqp.Endpoint{}
	for _, thisUrl := range strings.Split(brokerUrl, ",") {
		parsedURL, err := url.Parse(thisUrl)
		if err != nil {
			return nil, err
		}

		tlsConfig := &tls.Config{
			ServerName: parsedURL.Hostname(),
		}
		endpoints = append(endpoints, amqp.Endpoint{Address: thisUrl, Options: &amqp.AmqpConnOptions{TLSConfig: tlsConfig}})
	}
	env := amqp.NewClusterEnvironment(endpoints)

	conn, err := env.NewConnection(context.Background())
	if err != nil {
		return nil, fmt.Errorf("Error opening connection to Broker with %s", err)
	}

	exchangeName, ok := os.LookupEnv(amqpBrokerExchange)
	if !ok {
		return nil, fmt.Errorf("%s environment variable not set", amqpBrokerExchange)
	}
	_, err = conn.Management().DeclareExchange(
		context.Background(), &amqp.FanOutExchangeSpecification{Name: exchangeName})
	if err != nil {
		return nil, fmt.Errorf("Error declaring exchange %s with %s", exchangeName, err)
	}

	pub, err := conn.NewPublisher(context.Background(), &amqp.ExchangeAddress{
		Exchange: exchangeName,
	}, nil)

	return &Broker{
		environment: env,
		publisher:   pub,
	}, nil
}

func (broker *Broker) Send(ctx context.Context, event *activity.Event) (*activity.Event, error) {

	jsonMsg, err := json.Marshal(event)
	result, err := broker.publisher.Publish(context.Background(), amqp.NewMessage(jsonMsg))
	if err != nil {
		return nil, err
	}
	switch result.Outcome.(type) {
	case *amqp.StateAccepted:
		return event, nil
	}
	return nil, nil //should be unreachable.
}
