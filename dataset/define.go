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

type ModelGet interface {
	Get(string) any
}
type ModelSet interface {
	Set(k string, v any) error
}

type ModelClone interface {
	Clone() any
}
