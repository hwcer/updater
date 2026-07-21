package dataset

import (
	"fmt"
	"testing"
)

type role struct {
	Id   string
	Lv   int32 `json:"lv" bson:"lv"`
	Name string
}

// testCollectionWriter 记录 Save 期间产生的持久化操作,替代真实数据库
type testCollectionWriter struct {
	deleted  []any
	inserted []any
	updated  map[string]Update
}

func (w *testCollectionWriter) Delete(where ...any) {
	w.deleted = append(w.deleted, where...)
}

func (w *testCollectionWriter) Insert(documents ...any) {
	w.inserted = append(w.inserted, documents...)
}

func (w *testCollectionWriter) Setter(_id string, dirty Update, unset []string) error {
	if w.updated == nil {
		w.updated = make(map[string]Update)
	}
	w.updated[_id] = dirty
	return nil
}

func TestName(t *testing.T) {
	player := NewColl()
	for i := int32(1); i < 100; i++ {
		id := fmt.Sprintf("%v", i)
		player.Receive(id, &role{
			Id:   id,
			Lv:   i,
			Name: "Name-" + id,
		})
	}
	k := "1"
	doc, _ := player.Get(k)
	t.Logf("%+v", doc.data)
	if err := player.Update(k, Update{"lv": 100}); err != nil {
		t.Logf("Update Err:%v", err)
	}
	t.Logf("lv:%v", doc.GetInt32("lv"))
	w := &testCollectionWriter{}
	if err := player.Save(w); err != nil {
		t.Fatalf("Save Err:%v", err)
	}
	if v := ParseInt64(w.updated[k]["lv"]); v != 100 {
		t.Fatalf("Setter dirty error,lv:%v", v)
	}
	t.Logf("修改结果：%+v", doc.data)
	player.Release()
}
