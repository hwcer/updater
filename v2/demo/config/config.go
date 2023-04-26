package config

//模拟配置文件

var Configs = map[int32]*Config{}

type Config struct {
	Id    int32
	IMax  int64
	IType int32
}

func IMax(iid int32) (r int64) {
	if v := Configs[iid]; v != nil {
		r = v.IMax
	}
	return
}

func IType(iid int32) (r int32) {
	if v := Configs[iid]; v != nil {
		r = v.IType
	}
	//fmt.Printf("IType iid:%v ==== %v\n", iid, r)
	return
}

func Register(iid int32, iType int32, imax int64) {
	//fmt.Printf("Register iid:%v ==== %v\n", iid, iType)
	Configs[iid] = &Config{Id: iid, IMax: imax, IType: iType}
}
