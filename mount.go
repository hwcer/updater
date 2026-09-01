package updater

// 临时句柄挂载：为"Updater 之外、但要与玩家数据同批次原子写库"的数据准备。
//
// 典型场景：邮件领取标记、兑换码占用、临时战斗副本。它们的共同点是
// 要同批次原子写库 + 按需查库 + 可选内存驻留，**不要 IType 路由、不要 operator 自动生成**。
//
// 设计取舍见 HANDLER_MOUNT_PLAN.md。

// Mount 挂载/取回一个临时数据集合，keys 非空时顺带把这几条**当场查出来**。
//
// 幂等：同模型重复 Mount 直接返回已挂句柄 —— 长命场景（战斗副本）的每个 handler 开头
// 都是这一行，首个请求创建、后续全是复用，业务不必自己记"挂没挂过"。
//
//	coll, err := u.Mount(&model.Battle{}, battleId) //挂载 + 取数，一行搞定
//	coll, err := u.Mount(&model.Mail{}, ids...)     //多条一起
//	coll, err := u.Mount(&model.Battle{})           //只挂载，稍后自己 Select + Data
//
// keys 是文档 _id（string）—— 临时集合不进 IType 路由，没有 iid 这个概念。
//
// 带 keys 时等价于 Select(keys...) + Data()，**当场查库**（不等框架的 Data 阶段）——
// 临时数据取回来就是要马上用的，分两步写只是多一次出错的机会。
// 已在内存里的 key 会被 Select 跳过，所以长命句柄反复这么调不会重复查库。
//
// 挂载名取 model.TableName()，与已注册的全局模型重名时报错：撞名的话同一张表在一个
// Updater 里会有两个句柄各写各的，是静默的数据竞争。
//
// ⚠️ 不带 keys 时**不做任何预加载**。别指望它像全局句柄那样开场全量拉 ——
// 那对战斗副本这类表是灾难。
//
// ⚠️ **挂载与取数是两码事**：查库失败时返回 (句柄, err) —— 句柄已经挂上且完全可用，
// 只是这几条没读回来，重试一次 Select + Data 即可。唯一会返回 nil 的是重名，
// 那时压根没挂上。
func (u *Updater) Mount(model MountModel, keys ...string) (*MountCollection, error) {
	name := model.TableName()
	r, exist := u.mounts[name]
	if !exist {
		for _, m := range modelsRank {
			if m.name == name {
				return nil, Errorf(0, "mount name conflicts with registered model:%v", name)
			}
		}
		if u.mounts == nil {
			u.mounts = make(map[string]*MountCollection)
		}
		r = newMountCollection(u, model)
		r.reset()
		u.mounts[name] = r
	}
	r.unmount = false //改主意了:上一次标记的卸载作废
	if len(keys) == 0 {
		return r, nil
	}
	for _, k := range keys {
		r.Select(k)
	}
	return r, r.Data()
}

// Mounted 取回已挂载的临时集合，未挂载返回 nil。
//
// 与 Mount 的区别：它**只取不挂**，也不取数——用来问"挂了没"。
// 三个入口统一收 MountModel，业务层不必关心挂载名是怎么来的。
func (u *Updater) Mounted(model MountModel) *MountCollection {
	return u.mounts[model.TableName()]
}

// Unmount 标记卸载。**只打标记，真正摘除在 Release 阶段**（EventTypeRelease 之后）。
//
// 🔴 不在这里直接刷盘/摘除，是为了让短流程也走完整生命周期。短命场景的标准写法是
//
//	coll, err := u.Mount(&model.Mail{}, ids...)
//	defer u.Unmount(&model.Mail{})
//
// 而 defer 在 **handler 返回时**执行，框架的 Submit 排在那之后
// （`Updater.Verify` 的注释里写着"handle 返回后框架才 Submit"）。当场摘除的话，
// 这次改动就永远写不出去且一声不吭；退一步在 Unmount 里自己 save 也不对 ——
// 那是绕开 submit 另开一条旁路，与全局句柄的路径不一致，以后 submit 上加的任何东西
// 短流程都吃不到。
//
// 打完标记后句柄照旧留在 mounts 里，正常参与 Data / verify / submit，
// 直到请求结束才被摘掉 —— 长短两档走的是同一条路。
//
// ⚠️ 标记可撤销：同一请求内再次 Mount 同一模型会清掉它（业务改主意了，句柄还给它）。
//
// ⚠️ 卸载粒度是**整张表**，不是"这一条"。同一模型上并发多个实例（一个玩家两场战斗）时，
// 先结束的那场 Unmount 会把另一场的内存一起端掉 —— 数据不会丢（每次 Submit 都写穿了库），
// 但对方后续 Get 会拿到 nil 直到重新 Select。口径：**只有最后一个实例结束时才 Unmount**；
// 业务判断不了是不是最后一个，就别卸，留给玩家下线兜底（Destroy 会刷盘）。
func (u *Updater) Unmount(model MountModel) {
	if r, ok := u.mounts[model.TableName()]; ok {
		r.unmount = true
	}
}
