// Package schema defines the database schema (docs/adr/0008).
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
	"github.com/google/uuid"
)

// IDMixin gives a schema a random UUID primary key.
type IDMixin struct{ mixin.Schema }

// Fields of the mixin.
func (IDMixin) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
	}
}

// TimeMixin adds creation and update timestamps.
type TimeMixin struct{ mixin.Schema }

// Fields of the mixin.
func (TimeMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
