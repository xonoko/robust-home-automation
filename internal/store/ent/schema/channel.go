package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Channel is one capability of a device with its protocol configuration,
// e.g. a KNX group address, a Modbus register or a Zigbee2MQTT attribute.
type Channel struct{ ent.Schema }

// Mixin of Channel.
func (Channel) Mixin() []ent.Mixin { return []ent.Mixin{IDMixin{}, TimeMixin{}} }

// Fields of Channel.
func (Channel) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("device_id", uuid.UUID{}),
		field.String("key").NotEmpty().Comment("Stable identifier of the channel within its device"),
		field.String("kind").NotEmpty().Comment("Capability, e.g. switch, dimmer, temperature"),
		field.JSON("config", map[string]any{}).Optional().Comment("Protocol-specific settings"),
	}
}

// Edges of Channel.
func (Channel) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("device", Device.Type).Ref("channels").Field("device_id").Unique().Required(),
		edge.From("entities", Entity.Type).Ref("channels"),
	}
}

// Indexes of Channel.
func (Channel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("device_id", "key").Unique(),
	}
}
