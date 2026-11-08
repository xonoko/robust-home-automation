package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Device is a physical device reached through an integration,
// e.g. a KNX actuator, a Modbus heat pump or a Zigbee sensor.
type Device struct{ ent.Schema }

// Mixin of Device.
func (Device) Mixin() []ent.Mixin { return []ent.Mixin{IDMixin{}, TimeMixin{}} }

// Fields of Device.
func (Device) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("integration").NotEmpty().Comment("Integration instance that owns the device"),
		field.String("integration_ref").NotEmpty().Comment("Device identifier inside the integration"),
		field.String("manufacturer").Optional(),
		field.String("model").Optional(),
		field.UUID("area_id", uuid.UUID{}).Optional().Nillable(),
	}
}

// Edges of Device.
func (Device) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("area", Area.Type).Ref("devices").Field("area_id").Unique(),
		edge.To("channels", Channel.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

// Indexes of Device.
func (Device) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("integration", "integration_ref").Unique(),
	}
}
