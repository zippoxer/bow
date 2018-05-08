package bow

import (
	"fmt"
	"reflect"
	"sync"
)

var (
	// structCache is a cache of types to their key field index.
	structCache   = make(map[reflect.Type]int)
	structCacheMu sync.RWMutex

	typeOfId = reflect.TypeOf(Id{})
)

type structType struct {
	typ  reflect.Type
	ki   int
	ptrs int
}

func newStructType(v interface{}, mustAddr bool) (*structType, error) {
	typ := reflect.TypeOf(v)
	var kind reflect.Kind
	var ptrs int
	for {
		kind = typ.Kind()
		if kind == reflect.Struct {
			break
		}
		if kind != reflect.Ptr {
			return nil, fmt.Errorf("type %s is not a struct", typ)
		}
		typ = typ.Elem()
		ptrs++
	}
	if mustAddr && ptrs == 0 {
		return nil, fmt.Errorf(
			"type %s is not addressable, did you forget to pass a pointer?", typ)
	}
	return &structType{typ: typ, ptrs: ptrs, ki: -2}, nil
}

func (t *structType) keyIndex() (int, error) {
	if t.ki != -2 {
		return t.ki, nil
	}
	structCacheMu.RLock()
	fieldIndex, ok := structCache[t.typ]
	structCacheMu.RUnlock()
	if ok {
		return fieldIndex, nil
	}
	fieldIndex = -1
	for i := 0; i < t.typ.NumField(); i++ {
		field := t.typ.Field(i)
		if field.Type == typeOfId {
			fieldIndex = i
			break
		}
		flag, ok := field.Tag.Lookup("bow")
		if !ok {
			continue
		}
		switch flag {
		case "key":
			fieldIndex = i
			break
		}
	}
	t.ki = fieldIndex
	structCacheMu.Lock()
	structCache[t.typ] = fieldIndex
	structCacheMu.Unlock()
	return fieldIndex, nil
}

func (t *structType) value(v interface{}) *structValue {
	value := reflect.ValueOf(v)
	for i := 0; i < t.ptrs; i++ {
		value = value.Elem()
	}
	return &structValue{
		typ:   t,
		value: value,
	}
}

type structValue struct {
	typ   *structType
	value reflect.Value
}

func (v *structValue) key() ([]byte, error) {
	fieldIndex, err := v.typ.keyIndex()
	if err != nil {
		return nil, err
	}
	if fieldIndex == -1 {
		return nil, nil
	}
	key := v.value.Field(fieldIndex).Interface()
	return keyCodec.Marshal(key, nil)
}

func (v *structValue) setKey(key []byte) error {
	ki, err := v.typ.keyIndex()
	if err != nil {
		return err
	}
	if ki == -1 {
		return nil
	}
	field := v.value.Field(ki).Addr().Interface()
	return keyCodec.Unmarshal(key, field)
}
