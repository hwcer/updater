package dirty

type Update map[string]any

func NewUpdate(k string, v any) Update {
	if k == "" {
		return ParseUpdate(v)
	} else {
		return Update{k: v}
	}
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
