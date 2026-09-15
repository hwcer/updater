package updater

import "github.com/hwcer/updater/hamster"

// Handle 数据句柄接口（核心版接口，别名保持引用兼容）。
//
// 句柄并入核心后，hamster 是该接口的唯一实现者集合，生命周期方法
// （save/reset/reload/loading/release/destroy/commit/verify）已回到**未导出**——
// 与主干一致，业务侧天然不可见、不可调。
type Handle = hamster.Handle
