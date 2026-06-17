package dataset

type Cursor struct {
	items   []*Document
	users   map[string]struct{}
	release func()
}

func NewCursor(dataset Dataset, release func()) *Cursor {
	items := make([]*Document, 0, len(dataset))
	for _, doc := range dataset {
		items = append(items, doc)
	}
	return &Cursor{items: items, users: map[string]struct{}{}, release: release}
}

func (c *Cursor) Range(offset, size int, handle func(*Document) bool) {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(c.items) {
		return
	}
	end := len(c.items)
	if size > 0 && offset+size < end {
		end = offset + size
	}
	for i := offset; i < end; i++ {
		if !handle(c.items[i]) {
			return
		}
	}
}

func (c *Cursor) Paging(page, size int, handle func(*Document) bool) {
	if page < 1 {
		page = 1
	}
	c.Range((page-1)*size, size, handle)
}

func (c *Cursor) Len() int {
	return len(c.items)
}

func (c *Cursor) Close(key string) {
	if c.closed() {
		return
	}
	delete(c.users, key)
	if len(c.users) == 0 {
		c.items = nil
		c.users = nil
		if c.release != nil {
			c.release()
		}
	}
}

func (c *Cursor) closed() bool {
	return c.items == nil
}

type cursorMonitor struct {
	cursor *Cursor
}

func (m *cursorMonitor) Insert(doc *Document) {
	m.cursor.items = append(m.cursor.items, doc)
}

func (m *cursorMonitor) Delete(doc *Document) {
}
