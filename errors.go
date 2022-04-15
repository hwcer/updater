package updater

import "fmt"

type ErrMsg struct {
	msg  interface{}
	args []interface{}
}

func (e *ErrMsg) Error() string {
	if len(e.args) > 0 {
		return fmt.Sprintf("%v：%v", e.msg, e.args)
	} else {
		return fmt.Sprintf("%v", e.msg)
	}
}
func NewError(msg interface{}, args ...interface{}) *ErrMsg {
	return &ErrMsg{msg: msg, args: args}
}

func ErrItemNotExist(iid int32) *ErrMsg {
	return NewError("Item Not Exist", iid)
}

func ErrItemNotEnough(args ...interface{}) *ErrMsg {
	return NewError("Item Not Enough", args...)
}

func ErrITypeNotExist(iid int32) *ErrMsg {
	return NewError("IType Not Exist", iid)
}
func ErrDataNotExist(oid string) *ErrMsg {
	return NewError("Data Not Exist", oid)
}

var (
	ErrFieldNotExist = NewError("field not exist")
	ErrActValIllegal = NewError("act val illegal")
)
