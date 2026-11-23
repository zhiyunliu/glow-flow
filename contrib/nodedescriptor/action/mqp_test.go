package action

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/zhiyunliu/glue/queue"
	"github.com/zhiyunliu/golibs/xtypes"
)

type queueRecord struct {
	queueName string
	key       string
	value     any
	delay     int64
}

type fakeQueue struct {
	name    string
	records []queueRecord
	err     error
}

func (q *fakeQueue) Send(ctx context.Context, key string, value any) error {
	q.records = append(q.records, queueRecord{queueName: q.name, key: key, value: value})
	return q.err
}

func (q *fakeQueue) BatchSend(ctx context.Context, key string, value ...any) error {
	return errors.New("not implemented")
}

func (q *fakeQueue) DelaySend(ctx context.Context, key string, value any, delaySeconds int64) error {
	q.records = append(q.records, queueRecord{queueName: q.name, key: key, value: value, delay: delaySeconds})
	return q.err
}

func useFakeQueue(t *testing.T, fake *fakeQueue) {
	old := queueResolver
	queueResolver = func(name string) queue.IQueue {
		fake.name = name
		return fake
	}
	t.Cleanup(func() { queueResolver = old })
}

func TestMsgQueueProducer(t *testing.T) {
	fake := &fakeQueue{}
	useFakeQueue(t, fake)

	msg := orderMessage()
	send := buildNode(t, MsgQueueProducerNodeType, xtypes.XMap{"queuename": "orders", "queuekey": "order:${status}"})
	if result := execute(t, send, msg); result.RelationType != "success" || result.Msg != msg {
		t.Fatalf("send result = %+v", result)
	}

	delayed := buildNode(t, MsgQueueProducerNodeType, xtypes.XMap{"queuekey": "order:delay", "delay": "1500ms"})
	execute(t, delayed, msg)

	want := []queueRecord{
		{queueName: "orders", key: "order:paid", value: msg.Data},
		{queueName: defaultClientName, key: "order:delay", value: msg.Data, delay: 2},
	}
	if !reflect.DeepEqual(fake.records, want) {
		t.Fatalf("records = %#v, want %#v", fake.records, want)
	}

	fake.err = errors.New("broker down")
	assertFailure(t, execute(t, send, msg), msg)

	assertInvalidConfig(t, MsgQueueProducerNodeType, xtypes.XMap{"queuename": "orders"})
	assertInvalidConfig(t, MsgQueueProducerNodeType, xtypes.XMap{"queuekey": "k", "delay": "x"})
}
