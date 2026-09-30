package inapp

import "github.com/xraph/herald/driver"

var _ driver.Describer = (*Driver)(nil)

// Fields is empty: the in-app driver needs no configuration. That's a
// different answer from a driver that doesn't describe itself at all.
func (d *Driver) Fields() []driver.Field { return []driver.Field{} }
