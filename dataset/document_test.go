package dataset

import (
	"fmt"
	"strings"
	"testing"
)

// 定义一个结构体
type ExampleStruct struct {
	Field1 []int
	Field2 string
}

func TestDocument(t *testing.T) {
	src := &ExampleStruct{Field1: []int{1}, Field2: "test"}
	doc := NewDoc(src)
	copied := doc.Clone()

	e := copied.Any().(*ExampleStruct)
	fmt.Println("src:", src.Field1, src.Field2)
	fmt.Println("copied:", e.Field1, e.Field2)

	e.Field1[0] = 2
	e.Field2 = "test2"
	fmt.Println("src:", src.Field1, src.Field2)
	fmt.Println("copied:", e.Field1, e.Field2)
}

// nilSetDoc 模拟"只认单段键"的业务 handle：解析不了的 key 就 return nil,true
// （项目里的 map 子键 handle 普遍是这个写法：strconv.Atoi 失败即 return nil,true）
type nilSetDoc struct {
	Relics map[int32]int64
}

func (m *nilSetDoc) Set(k string, v any) (any, bool) {
	if strings.Contains(k, ".") {
		return nil, true // 多级路径解析不了，但仍声称"已处理"
	}
	return v, true
}

// 业务 handle 声称处理却没产出值时，不能把 nil 写进 dirty。
//
// 否则 cosmo 会原样发出 $set:{"relics.1.lv":null}，把库里的真实字段清成 null，
// 而内存毫不知情——重新加载后数据就没了。
func TestDocumentSetterRejectsNilFromModel(t *testing.T) {
	d := &nilSetDoc{}
	doc := NewDoc(d)

	doc.Set("relics.1.lv", int64(5)) // 多级路径，handle 会 return nil,true
	dirty, _ := doc.Save()
	if _, ok := dirty["relics.1.lv"]; ok {
		t.Fatalf("业务产出 nil 的键不该进 dirty，否则会把库里字段 $set 成 null：%v", dirty)
	}

	// 显式写 nil（清字段）仍要放行：v 本身就是 nil，不属于被拦的组合
	doc2 := NewDoc(&nilSetDoc{})
	doc2.Set("relics", nil)
	dirty2, _ := doc2.Save()
	if _, ok := dirty2["relics"]; !ok {
		t.Fatalf("显式写 nil 应正常进 dirty：%v", dirty2)
	}
}
