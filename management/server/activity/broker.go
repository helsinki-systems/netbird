package activity

import (
	"context"
)

type Broker interface {
	//Send activity event to message broker
	Send(ctx context.Context, event *Event) (*Event, error) 
}

