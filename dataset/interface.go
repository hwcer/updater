package dataset

// IModel 获取属性
type IModel interface {
	Get(key string) any
	Set(key string, val any) (any, error)
	Incr(key string, val int64) (r int64, err error)
	Copy() IModel
}
