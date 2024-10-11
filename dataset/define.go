package dataset

const (
	ItemNameOID = "_id"
	ItemNameVAL = "val"
)

type Model interface {
	GetOID() string //获取OID
	GetIID() int32  //获取IID
}

type ModelGet interface {
	Get(string) (v any, ok bool)
}
type ModelSet interface {
	Set(k string, v any) (r any, ok bool)
}

//type ModelUnset interface {
//	Unset(k string) (ok bool)
//}

type ModelClone interface {
	Clone() any
}

//type ModelSaving interface {
//	Saving(map[string]any)
//}

type BulkWrite interface {
	Save() error
	Update(data any, where ...any)
	Insert(documents ...any)
	Delete(where ...any)
}
