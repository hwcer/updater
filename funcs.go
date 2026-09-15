package updater

import (
	"github.com/hwcer/updater/dataset"
)

// docIID 取文档的数值分组键：优先 dataset.Model.GetIID()，回落 Fields.IID 字段名约定。
// （核心 hamster 侧同名逻辑用于溢出分组；这里供挂载包装的 Count 统计使用）
func docIID(doc *dataset.Document) int32 {
	if m, ok := doc.Any().(dataset.Model); ok {
		return m.GetIID()
	}
	return doc.GetInt32(dataset.Fields.IID)
}
