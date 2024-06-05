package key

import (
	"encoding"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
)

func textMarshalBson(val encoding.TextMarshaler) (bsontype.Type, []byte, error) {
	textBytes, err := val.MarshalText()
	if err != nil {
		return 0, nil, err
	}

	return bson.MarshalValue(string(textBytes))
}

func textUnmarshalBson(val encoding.TextUnmarshaler, b bsontype.Type, bytes []byte) error {
	var s = new(string)

	if err := bson.UnmarshalValue(b, bytes, s); err != nil {
		return err
	}

	return val.UnmarshalText([]byte(*s))
}

func (n *NodePublic) MarshalBSONValue() (bsontype.Type, []byte, error) {
	return textMarshalBson(n)
}

func (n *NodePublic) UnmarshalBSONValue(b bsontype.Type, bytes []byte) error {
	return textUnmarshalBson(n, b, bytes)
}

func (s *SessionPublic) MarshalBSONValue() (bsontype.Type, []byte, error) {
	return textMarshalBson(s)
}

func (s *SessionPublic) UnmarshalBSONValue(b bsontype.Type, bytes []byte) error {
	return textUnmarshalBson(s, b, bytes)
}

func (c *ControlPublic) MarshalBSONValue() (bsontype.Type, []byte, error) {
	return textMarshalBson(c)
}

func (c *ControlPublic) UnmarshalBSONValue(b bsontype.Type, bytes []byte) error {
	return textUnmarshalBson(c, b, bytes)
}
