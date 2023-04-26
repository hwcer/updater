package model

func NewIType(id int32) *IType {
	return &IType{id: id}
}

type IType struct {
	id int32
}

func (this *IType) Id() int32 {
	return this.id
}
