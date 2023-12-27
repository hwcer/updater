package model

import (
	"github.com/hwcer/cosgo/utils"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater"
	"github.com/hwcer/updater/dataset"
	"github.com/hwcer/updater/demo/config"
	"github.com/hwcer/updater/operator"
)

const ticketPlugName = "_TicketPlugName"

type ticketLimit []int32

type cycleHandle func(dateTime *utils.DateTime, powerTime int64, powerMax int64, cycle int64) (addVal int64, newTime int64)

var cycleHandleDict = make(map[int32]cycleHandle)

var TicketIType = &ticketIType{}

func init() {
	cycleHandleDict[1] = cycleHandleType1
	cycleHandleDict[2] = cycleHandleType2
	TicketIType.id = config.ITypeTicket
	ItemIType.register(TicketIType)
}

type ticketConfig interface {
	GetDot() []int32
	GetCycle() []int32
	GetLimit() []int32
}

type ticketConfigDefault struct {
	id int32
}

func (this ticketConfigDefault) GetDot() []int32 {
	return []int32{10, 1} //每10秒回复一点
}

func (this ticketConfigDefault) GetCycle() []int32 {
	return []int32{1, 100} //每天回复100
}
func (this ticketConfigDefault) GetLimit() []int32 {
	return []int32{0, 0, 100} //[iid,x,y]  最大值 = (x * val(iid))/10000 + y
}

type ticketIType struct {
	itemIType
}

func (this *Item) GetPowerTime() (r int64) {
	_ = this.GetAttach(&r)
	return
}
func (this *Item) SetPowerTime(t int64) {
	_ = this.SetAttach(t)
}

// Listener 自动回复
func (this *ticketIType) Listener(u *updater.Updater, op *operator.Operator) {
	c := &ticketConfigDefault{id: op.IID}
	limit := c.GetLimit()
	if limit[0] > 0 && limit[1] > 0 {
		u.Select(limit[0])
	}
	plug := u.Events.LoadOrStore(ticketPlugName, &ticketPlug{}).(*ticketPlug)
	plug.add(op.IID, c)
}

type ticketPlug struct {
	dict map[int32]ticketConfig
}

func (this *ticketPlug) Emit(u *updater.Updater, t updater.EventType) error {
	if t == updater.EventTypePreVerify && len(this.dict) > 0 {
		return this.checkAllTicket(u)
	}
	return nil
}
func (this *ticketPlug) add(iid int32, c ticketConfig) {
	if this.dict == nil {
		this.dict = map[int32]ticketConfig{}
	}
	this.dict[iid] = c
}

func (this *ticketPlug) checkAllTicket(u *updater.Updater) error {
	for iid, c := range this.dict {
		if v := u.Get(iid); v != nil {
			this.sumTicket(u, c, v.(*Item))
		} else {
			this.newTicket(u, c, iid)
		}
	}
	return nil
}

func (this *ticketPlug) powerMax(u *updater.Updater, c ticketConfig, iid int32) int64 {
	limit := c.GetLimit()
	powerMax := int64(limit[2])
	if limit[0] > 0 && limit[1] > 0 {
		powerMax += u.Val(limit[0]) * int64(limit[1]) / 10000
	}
	return powerMax
}

func (this *ticketPlug) newTicket(u *updater.Updater, c ticketConfig, iid int32) {
	i := &Item{}
	if oid, err := TicketIType.ObjectId(u, iid); err == nil {
		i.OID = oid
	} else {
		logger.Debug("Ticket ObjectId error:%v", err)
		return
	}
	i.Uid = u.Uid()
	i.IID = iid
	i.Val = this.powerMax(u, c, iid)
	i.SetPowerTime(u.Time.Unix())
	_ = u.New(i, true)
}

func (this *ticketPlug) sumTicket(u *updater.Updater, c ticketConfig, data *Item) {
	t := utils.Time.New(u.Time)
	nowTime := t.Now().Unix()
	powerMax := this.powerMax(u, c, data.IID)
	update := dataset.Update{}
	powerTime := data.GetPowerTime()

	if data.Val >= powerMax {
		update["attach"] = nowTime
	} else if powerTime == 0 {
		//初始回满
		//att.Set(ItemAttachTypeTicketUms, nowTime)
		update["attach"] = nowTime
		if data.Val < powerMax {
			update["val"] = powerMax
		}
	} else {
		var addVal int64
		var newTime int64
		//每日，周回复
		cycle := c.GetCycle()
		if f := cycleHandleDict[cycle[0]]; f != nil {
			addVal, newTime = f(t, powerTime, powerMax, int64(cycle[1]))
		}
		//计时回复
		dot := c.GetDot()
		if powerTime < nowTime && dot[0] > 0 && dot[1] > 0 {
			dotNum := int64(dot[0])
			diffTime := nowTime - powerTime
			retNum := diffTime / dotNum * int64(dot[1])
			if retNum > 0 {
				lastTime := powerTime + retNum*dotNum
				if lastTime > newTime {
					newTime = lastTime
				}
				addVal += retNum
			}
		}
		if newTime > 0 {
			update["attach"] = newTime
		}
		if addVal > 0 {
			newVal := data.Val + addVal
			if newVal > powerMax {
				update["val"] = powerMax
			} else {
				update["val"] = newVal
			}
		}

	}
	if len(update) > 0 {
		op := &operator.Operator{}
		op.OID = data.OID
		op.IID = data.IID
		op.Type = operator.TypesSet
		op.Result = update
		if err := u.Operator(op, true); err != nil {
			logger.Alert(err)
		}
	}
}

// 每日回复
func cycleHandleType1(t *utils.DateTime, powerTime int64, powerMax, cycle int64) (addVal int64, newTime int64) {
	lastTime := t.Daily(0).Unix()
	if powerTime >= lastTime {
		return
	}
	newTime = lastTime
	if cycle == 0 {
		addVal = powerMax
	} else {
		for ; powerTime < lastTime; powerTime += 86400 {
			addVal += cycle
		}
	}
	return
}

// 每周回复
func cycleHandleType2(t *utils.DateTime, powerTime int64, powerMax, cycle int64) (addVal int64, newTime int64) {
	lastTime := t.Weekly(0).Unix()
	if powerTime >= lastTime {
		return
	}
	newTime = lastTime
	var weekSecond int64 = 86400 * 7
	if cycle == 0 {
		addVal = powerMax
	} else {
		for ; powerTime < lastTime; lastTime += weekSecond {
			addVal += cycle
		}
	}
	return
}
