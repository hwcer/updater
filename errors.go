package updater

import (
	"fmt"
	"github.com/hwcer/updater/operator"
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
func ErrIidEmpty(op *operator.Operator) *ErrMsg {
	return NewError("iid empty:%+v", op)
}
func ErrITypeNotExist(iid int32) *ErrMsg {
	return NewError("IType Not Exist", iid)
}

func ErrOIDEmpty(args ...any) *ErrMsg {
	return NewError("oid empty", args...)
}

var (
	ErrUnableUseIIDOperation = NewError("unable to use iid operation")
)
