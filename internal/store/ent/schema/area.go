package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Area is a floor, room or zone. Areas nest: a room belongs to a floor.
type Area struct{ ent.Schema }

// Mixin of Area.
func (Area) Mixin() []ent.Mixin { return []ent.Mixin{IDMixin{}, TimeMixin{}} }

// Fields of Area.
func (Area) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.Enum("kind").Values("floor", "room", "zone").Default("room"),
		field.UUID("parent_id", uuid.UUID{}).Optional().Nillable(),
	}
}

// Edges of Area.
func (Area) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("children", Area.Type).From("parent").Field("parent_id").Unique(),
		edge.To("devices", Device.Type),
		edge.To("entities", Entity.Type),
	}
}
