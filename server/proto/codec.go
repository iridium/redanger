package proto

import (
	"fmt"
	"reflect"
)

type Decoder interface {
	Decode(r *Reader) error
}

type Encoder interface {
	Encode(b *Body)
}

func Unmarshal(r *Reader, v any) error {
	if d, ok := v.(Decoder); ok {
		return d.Decode(r)
	}

	rv := reflect.ValueOf(v).Elem()
	rt := rv.Type()

	for i := range rt.NumField() {
		field := rv.Field(i)
		switch field.Kind() {
		case reflect.Int16:
			val, err := r.ReadShort()
			if err != nil {
				return fmt.Errorf("field %s: %w", rt.Field(i).Name, err)
			}
			field.SetInt(int64(val))
		case reflect.Int32:
			val, err := r.ReadInt()
			if err != nil {
				return fmt.Errorf("field %s: %w", rt.Field(i).Name, err)
			}
			field.SetInt(int64(val))
		case reflect.String:
			val, err := r.ReadString()
			if err != nil {
				return fmt.Errorf("field %s: %w", rt.Field(i).Name, err)
			}
			field.SetString(val)
		case reflect.Slice:
			if field.Type().Elem().Kind() == reflect.Uint8 {
				val, err := r.ReadBinary()
				if err != nil {
					return fmt.Errorf("field %s: %w", rt.Field(i).Name, err)
				}
				field.SetBytes(val)
			}
		}
	}
	return nil
}

func Marshal(v any) *Body {
	var b Body
	if e, ok := v.(Encoder); ok {
		e.Encode(&b)
		return &b
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	rt := rv.Type()

	for i := range rt.NumField() {
		field := rv.Field(i)
		switch field.Kind() {
		case reflect.Int16:
			b.WriteShort(int16(field.Int()))
		case reflect.Int32:
			b.WriteInt(int32(field.Int()))
		case reflect.String:
			b.WriteString(field.String())
		case reflect.Slice:
			if field.Type().Elem().Kind() == reflect.Uint8 {
				b.WriteBinary(field.Bytes())
			}
		}
	}
	return &b
}
