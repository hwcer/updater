package updater

import "fmt"

//用于 存储临时Handle

type Handler map[string]Handle

func (h Handler) Has(name string) bool {
	_, ok := h[name]
	return ok
}

func (h Handler) Get(name string) any {
	return h[name]
}

func (h Handler) Delete(name string) {
	delete(h, name)
}

func (h Handler) GetOrCreate(u *Updater, name string, parser Parser, ram RAMType, model Model) (Handle, error) {
	if i, ok := h[name]; ok {
		return i, nil
	}
	nh := handles[parser]
	if nh == nil {
		u.Error = fmt.Errorf("no handler for %v", parser)
		return nil, u.Error
	}
	mod := Model{
		ram:    ram,
		name:   name,
		model:  model,
		parser: parser,
		order:  0,
	}
	hh := nh(u, &mod)
	if u.Error = hh.loading(); u.Error != nil {
		return nil, u.Error
	}
	hh.reset()

	h[name] = hh

	return hh, nil
}
