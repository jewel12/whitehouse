package function

import (
	"context"
	"log"

	"github.com/jewel12/whitehouse/img-generators/remo"
)

type PubSubMessage struct {
	Data []byte `json:"data"`
}

func GenRemoImg(ctx context.Context, m PubSubMessage) error {
	if err := remo.Load(); err != nil {
		log.Fatalln(err)
	}
	return nil
}
