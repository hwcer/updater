package config

const (
	ITypeRole   int32 = 10
	ITypeItem   int32 = 30
	ITypeHero   int32 = 40
	ITypeEquip  int32 = 50
	ITypeTicket int32 = 60
	ITypeDaily  int32 = 80
	ITypeRecord int32 = 90
)

func init() {
	//模拟生成配置
	itypes := []int32{ITypeItem, ITypeHero, ITypeEquip, ITypeTicket, ITypeDaily, ITypeRecord}
	for _, it := range itypes {
		for i := int32(1); i <= 1000; i++ {
			iid := it*10000 + i
			imax := int64(0)
			if it == ITypeHero {
				imax = 1
			}
			Register(iid, it, imax)
		}
	}
}
