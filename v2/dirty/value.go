package dirty

type Value map[any]any
type Update map[string]any

func NewValue(k, v any) Value {
	return Value{k: v}
}

func NewUpdate(k string, v any) Update {
	if k == "" {
		return ParseUpdate(v)
	} else {
		return Update{k: v}
	}
}

func ParseValue(src any) Value {
	switch v := src.(type) {
	case map[any]any:
		return v
	case Value:
		return v
	}
	return nil
}

func ParseUpdate(src any) Update {
	switch v := src.(type) {
	case map[string]any:
		return v
	case Update:
		return v
	}
	return nil
}
