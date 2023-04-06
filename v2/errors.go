package updater

import (
	"fmt"
	"github.com/hwcer/updater/v2/operator"
)

type ErrMsg struct {
	msg  interface{}
	args interface{}
}

func (e *ErrMsg) Error() string {
	if e.args != nil {
		return fmt.Sprintf("%v：%v", e.msg, e.args)
	} else {
		return fmt.Sprintf("%v", e.msg)
	}
}
func NewError(msg interface{}, args ...interface{}) *ErrMsg {
	return &ErrMsg{msg: msg, args: args}
}
func ErrArgsIllegal(args ...any) *ErrMsg {
	return NewError("args illegal", args...)
}

func ErrItemNotExist(id any) *ErrMsg {
	return NewError("Item Not Exist", id)
}

func ErrItemNotEnough(args ...interface{}) *ErrMsg {
	return NewError("Item Not Enough", args...)
}

func ErrITypeNotExist(iid int32) *ErrMsg {
	return NewError("IType Not Exist", iid)
}

//func ErrCreateIdUnknown(name string) *ErrMsg {
//	return NewError("IType ObjectID Unknown", name)
//}
//
//func ErrDataNotExist(oid string) *ErrMsg {
//	return NewError("Data Not Exist", oid)
//}
//func ErrKeyIllegal(id ikey) *ErrMsg {
//	return NewError("id illegal", id)
//}
//
//func ErrActValIllegal(act *Cache) *ErrMsg {
//	return NewError("act val illegal", act.Val)
//}

func ErrActKeyIllegal(act *operator.Operator) *ErrMsg {
	return NewError("act key illegal", act.IID)
}

var (
	ErrUidEmpty = NewError("user id empty")
	//ErrFieldNotExist    = NewError("field not exist")
	//ErrFieldNotExist    = NewError("field not exist")
	//ErrHashModelIllegal = NewError("hash mode illegal")

	//ErrActValIllegal = NewError("act val illegal")
)
