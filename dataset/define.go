package dataset

const (
	ItemNameOID = "_id"
	ItemNameVAL = "val"
)

type Model interface {
	GetOID() string //获取OID
	GetIID() int32  //获取IID
}
type ModelVal interface {
	GetVal() int64
}

type ModelGet interface {
	Get(string) (v any, ok bool)
}

func GetVal(i any) int64 {
	if mv, ok := i.(ModelVal); ok {
		return mv.GetVal()
	}
	if mg, ok := i.(ModelGet); ok {
		if v, exist := mg.Get(ItemNameVAL); exist {
			return ParseInt64(v)
		}
	}
	return 0
}

// ModelSet 内存写入
//
//	r.(type)==Update 时直接将 r.(Update)写入数据库
//	其他类型  写入{k:r}
type ModelSet interface {
	Set(k string, v any) (r any, ok bool)
}

type ModelClone interface {
	Clone() any
}

type BulkWrite interface {
	Submit() error
	Update(data Update, where ...any)
	Insert(documents ...any)
	Delete(where ...any)
	String() string
}
