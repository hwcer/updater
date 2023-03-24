package dataset

const (
	ItemNameOID = "_id"
	ItemNameIID = "iid"
	ItemNameVAL = "val"
	ItemNameUID = "uid"
)

type ModelGet interface {
	Get(string) any
}
type ModelSet interface {
	Set(k string, v any) error
}
