package glowflow

import (
	"reflect"
	"time"

	"github.com/zhiyunliu/glow-flow/constants/enums/datatype"
)

type Message struct {
	MsgId     string            `json:"msg_id"`
	Timestamp int64             `json:"timestamp"`
	DataType  datatype.DataType `json:"data_type"`
	Data      any               `json:"data"`
}

func NewMessage(msgId string, data any) *Message {
	return &Message{
		MsgId:     msgId,
		Timestamp: time.Now().Unix(),
		DataType:  getMessageDataType(data),
		Data:      data,
	}
}

func (m *Message) SetData(data any) {
	m.DataType = getMessageDataType(data)
	m.Data = data
}

func getMessageDataType(data any) datatype.DataType {
	refVal := reflect.ValueOf(data)
	for refVal.Kind() == reflect.Ptr {
		refVal = refVal.Elem()
	}

	switch refVal.Kind() {
	case reflect.String:
		return datatype.String
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return datatype.Int
	case reflect.Float32, reflect.Float64:
		return datatype.Float
	case reflect.Bool:
		return datatype.Bool
	case reflect.Map, reflect.Struct:
		return datatype.Object
	case reflect.Array, reflect.Slice:
		return datatype.Array
	default:
		return datatype.Any
	}
}
