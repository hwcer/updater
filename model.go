package updater

import (
	"fmt"
	"github.com/hwcer/cosgo/schema"
)

type Parser int8
type handleFunc func(updater *Updater, model any, ram RAMType) Handle

const (
	ParserTypeHash       Parser = iota //Map[string]int64模式
	ParserTypeDocument                 //Document 单文档模式
	ParserTypeCollection               //Collection 文档集合模式
)

var handles = make(map[Parser]handleFunc)

func init() {
	NewHandle(ParserTypeHash, NewHash)
	NewHandle(ParserTypeDocument, NewDocument)
	NewHandle(ParserTypeCollection, NewCollection)
}

// NewHandle 注册新解析器
func NewHandle(name Parser, f handleFunc) {
	handles[name] = f
}

var modelsRank []*Model
var modelsDict = make(map[int32]*Model)
var itypesDict = make(map[int32]IType) //ITypeId = IType

type Model struct {
	ram    RAMType
	name   string
	model  any
	parser Parser
}

func Register(parser Parser, ram RAMType, model any, itypes ...IType) error {
	if _, ok := handles[parser]; !ok {
		return fmt.Errorf("parser unknown:%v", parser)
	}
	mod := &Model{ram: ram, model: model, parser: parser}
	sch, err := schema.Parse(model)
	if err != nil {
		return err
	}
	mod.name = sch.Table
	modelsRank = append(modelsRank, mod)
	for _, it := range itypes {
		if parser == ParserTypeCollection {
			it = it.(ITypeCollection)
		}
		id := it.Id()
		if _, ok := modelsDict[id]; ok {
			return fmt.Errorf("model IType(%v)已经存在:%v", it, mod.name)
		}
		modelsDict[id] = mod
		itypesDict[id] = it
	}
	return nil
}
