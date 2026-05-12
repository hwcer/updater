package updater

import (
	"fmt"
	"sort"

	"github.com/hwcer/cosgo/schema"
)

type Parser int8

const (
	ParserTypeValues     Parser = iota //Map[string]int64模式
	ParserTypeDocument                 //Document 单文档模式
	ParserTypeCollection               //Collection 文档集合模式
	ParserTypeVirtual                  //Virtual 虚拟模式,本身不会存储数据，依赖于其他模块数据，如 日常 依赖 历史数据
)

type handleFunc func(updater *Updater, model *Model) Handle

var handles = make(map[Parser]handleFunc)

func init() {
	NewHandle(ParserTypeValues, NewValues)
	NewHandle(ParserTypeDocument, NewDocument)
	NewHandle(ParserTypeCollection, NewCollection)
	NewHandle(ParserTypeVirtual, NewVirtual)
}

type TableOrder interface {
	TableOrder() int32
}

type ModelLoading interface {
	Loading() RAMType
}

// ModelReset 返回true时 重新调用 model.Getter
type ModelReset interface {
	Reset(*Updater, int64) bool
}

// NewHandle 注册新解析器
func NewHandle(name Parser, f handleFunc) {
	handles[name] = f
}

var modelsRank []*Model
var modelsDict = make(map[int32]*Model)
var itypesDict = make(map[int32]IType) //ITypeId = IType

type Model struct {
	ram     RAMType
	name    string
	model   any
	parser  Parser
	order   int32   //倒序排列
	loading RAMType //加载时内存模式
}

func ITypes(f func(int32, IType) bool) {
	for k, it := range itypesDict {
		if !f(k, it) {
			break
		}
	}
}
func Models(f func(int32, any) bool) {
	for k, m := range modelsDict {
		if !f(k, m) {
			break
		}
	}
}
func Register(parser Parser, ram RAMType, model any, its ...IType) error {
	if _, ok := handles[parser]; !ok {
		return fmt.Errorf("parser unknown:%v", parser)
	}

	if err := verifyModel(parser, model); err != nil {
		return err
	}

	mod := &Model{ram: ram, model: model, parser: parser}
	if t, ok := model.(schema.Tabler); ok {
		mod.name = t.TableName()
	} else {
		mod.name = schema.Kind(model).Name()
	}
	if o, ok := model.(TableOrder); ok {
		mod.order = o.TableOrder()
	} else {
		mod.order = -1
	}
	if o, ok := model.(ModelLoading); ok {
		mod.loading = o.Loading()
	} else {
		mod.loading = RAMTypeNone
	}
	modelsRank = append(modelsRank, mod)
	sort.SliceStable(modelsRank, func(i, j int) bool {
		return modelsRank[i].order > modelsRank[j].order
	})

	for _, it := range its {
		if err := verifyIType(parser, mod.name, it); err != nil {
			return err
		}
		id := it.ID()
		if _, ok := modelsDict[id]; ok {
			return fmt.Errorf("model IType(%v)已经存在:%v", it, mod.name)
		}
		modelsDict[id] = mod
		itypesDict[id] = it
	}
	return nil
}

func verifyModel(parser Parser, model any) error {
	switch parser {
	case ParserTypeValues:
		if _, ok := model.(valuesModel); !ok {
			return fmt.Errorf("model %T does not implement valuesModel", model)
		}
	case ParserTypeDocument:
		if _, ok := model.(documentModel); !ok {
			return fmt.Errorf("model %T does not implement documentModel", model)
		}
	case ParserTypeCollection:
		if _, ok := model.(collectionModel); !ok {
			return fmt.Errorf("model %T does not implement collectionModel", model)
		}
	case ParserTypeVirtual:
		if _, ok := model.(virtualModel); !ok {
			return fmt.Errorf("model %T does not implement virtualModel", model)
		}
	}
	return nil
}

func verifyIType(parser Parser, name string, it IType) error {
	switch parser {
	case ParserTypeCollection:
		if _, ok := it.(ITypeCollection); !ok {
			return fmt.Errorf("IType(%d) does not implement ITypeCollection for model %s", it.ID(), name)
		}
	default:
		return nil
	}
	return nil
}
