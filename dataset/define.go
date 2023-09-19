package dataset

const (
	ItemNameOID = "_id"
	ItemNameIID = "iid"
	ItemNameVAL = "val"
	ItemNameUID = "uid"
)

type Model interface {
	GetOID() string //获取OID
	GetIID() int32  //获取IID
}

type ModelVal interface {
	GetVal() int64 //获取IID
}

type ModelGet interface {
	Get(string) (v any, ok bool)
}
type ModelSet interface {
	Set(k string, v any) (ok bool)
}

type ModelClone interface {
	Clone() any
}
