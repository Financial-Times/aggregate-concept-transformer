package concept

import (
	"context"

	"github.com/Financial-Times/aggregate-concept-transformer/sns"
	"github.com/stretchr/testify/mock"
)

type mockSNSClient struct {
	mock.Mock
	eventList []sns.Event
	err       error
}

func (c *mockSNSClient) PublishEvents(ctx context.Context, messages []sns.Event) error {
	if c.err != nil {
		return c.err
	}

	c.eventList = append(c.eventList, messages...)

	return nil
}
