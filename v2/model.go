package updater

import (
	"fmt"
	"github.com/hwcer/cosgo/schema"
)

type Parser string

const (
	ParserTypeHash       Parser = "hash"       //Map[string]int64模式
	ParserTypeDocument          = "document"   //Document 单文档模式
	ParserTypeCollection        = "collection" //Collection 文档集合模式
)

var handles = make(map[Parser]func(updater *Updater, model any, ram RAMType) Handle)

func init() {
	handles[ParserTypeHash] = NewHash
	handles[ParserTypeDocument] = NewDocument
	handles[ParserTypeCollection] = NewCollection
}

// NewHandle 注册新解析器
func NewHandle(name Parser, f func(adapter *Updater, model any, ram RAMType) Handle) {
	handles[name] = f
}

var modelsRank []*Model
var modelsDict = make(map[int32]*Model)
var itypesDict = make(map[int32]IType)

type Model struct {
	iModel
	Name string
}

type iModel interface {
	Parser() Parser
}

func Register(model iModel, itypes ...IType) error {
	parser := model.Parser()
	if _, ok := handles[parser]; !ok {
		return fmt.Errorf("parser unknown:%v", parser)
	}
	i := &Model{iModel: model}
	sch, err := schema.Parse(model)
	if err != nil {
		return err
	}
	i.Name = sch.Table
	modelsRank = append(modelsRank, i)
	for _, it := range itypes {
		id := it.Id()
		if _, ok := modelsDict[id]; ok {
			return fmt.Errorf("model IType(%v)已经存在:%v", it, i.Name)
		}
		modelsDict[id] = i
		itypesDict[id] = it
	}
	return nil
}
