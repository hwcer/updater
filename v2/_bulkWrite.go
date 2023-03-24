package updater

type BulkWrite interface {
	Save() (err error)
	Update(data interface{}, where ...interface{})
	Insert(documents ...interface{})
	Delete(where ...interface{})
}
