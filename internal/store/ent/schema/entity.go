package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Entity is the user-facing state and control (a light, a temperature, a blind).
// It links to one or more channels: a KNX light with separate switch, dim and
// status group addresses is one entity.
type Entity struct{ ent.Schema }

// Mixin of Entity.
func (Entity) Mixin() []ent.Mixin { return []ent.Mixin{IDMixin{}, TimeMixin{}} }

// Fields of Entity.
func (Entity) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("kind").NotEmpty().Comment("e.g. light, switch, sensor, cover, climate"),
		field.UUID("area_id", uuid.UUID{}).Optional().Nillable(),
	}
}

// Edges of Entity.
func (Entity) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("area", Area.Type).Ref("entities").Field("area_id").Unique(),
		edge.To("channels", Channel.Type),
	}
}
