package updater

type ModelNew interface {
	New() interface{}
}
type ModelCopy interface {
	Copy() interface{}
}

type ModelMakeSlice interface {
	MakeSlice() interface{}
}

//获取属性
type ModelGetVal interface {
	GetVal(key string) (interface{}, bool)
}

//设置属性
type ModelSetVal interface {
	SetVal(key string, val interface{}) error
}

//增加属性
type ModelAddVal interface {
	AddVal(key string, val int64) (r int64, err error)
}
