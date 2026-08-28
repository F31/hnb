package market

import (
	"fmt"
	"time"
)

type Channel struct {
	ID             string      `json:"id"`
	ProductID      string      `json:"product_id"`
	Name           string      `json:"name"`
	ChannelType    ChannelType `json:"channel_type"`
	ReleaseID      string      `json:"release_id,omitempty"`
	PromotionOrder int         `json:"promotion_order"`
	CreatedAt      time.Time   `json:"created_at"`
}

type ChannelPipeline struct {
	channels map[string]*Channel
}

func NewChannelPipeline() *ChannelPipeline {
	return &ChannelPipeline{
		channels: make(map[string]*Channel),
	}
}

func (cp *ChannelPipeline) AddChannel(channel *Channel) {
	cp.channels[channel.ID] = channel
}

func (cp *ChannelPipeline) Promote(channelID string, toType ChannelType, releaseID string) (*Channel, error) {
	ch, ok := cp.channels[channelID]
	if !ok {
		return nil, fmt.Errorf("channel %q not found", channelID)
	}

	if !CanPromote(ch.ChannelType, toType) {
		return nil, fmt.Errorf(
			"cannot promote from %s to %s", ch.ChannelType, toType,
		)
	}

	newCh := &Channel{
		ProductID:      ch.ProductID,
		Name:           fmt.Sprintf("%s-%s", ch.ProductID, toType),
		ChannelType:    toType,
		ReleaseID:      releaseID,
		PromotionOrder: channelOrder[toType],
		CreatedAt:      time.Now().UTC(),
	}

	return newCh, nil
}

func (cp *ChannelPipeline) GetChannel(channelID string) (*Channel, bool) {
	ch, ok := cp.channels[channelID]
	return ch, ok
}

func (cp *ChannelPipeline) GetReleaseForChannel(productID string, channelType ChannelType) (string, bool) {
	for _, ch := range cp.channels {
		if ch.ProductID == productID && ch.ChannelType == channelType && ch.ReleaseID != "" {
			return ch.ReleaseID, true
		}
	}
	return "", false
}
