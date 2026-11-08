-- Create "areas" table
CREATE TABLE "areas" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "name" character varying NOT NULL,
  "kind" character varying NOT NULL DEFAULT 'room',
  "parent_id" uuid NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "areas_areas_children" FOREIGN KEY ("parent_id") REFERENCES "areas" ("id") ON UPDATE NO ACTION ON DELETE SET NULL
);
-- Create "devices" table
CREATE TABLE "devices" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "name" character varying NOT NULL,
  "integration" character varying NOT NULL,
  "integration_ref" character varying NOT NULL,
  "manufacturer" character varying NULL,
  "model" character varying NULL,
  "area_id" uuid NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "devices_areas_devices" FOREIGN KEY ("area_id") REFERENCES "areas" ("id") ON UPDATE NO ACTION ON DELETE SET NULL
);
-- Create index "device_integration_integration_ref" to table: "devices"
CREATE UNIQUE INDEX "device_integration_integration_ref" ON "devices" ("integration", "integration_ref");
-- Create "channels" table
CREATE TABLE "channels" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "key" character varying NOT NULL,
  "kind" character varying NOT NULL,
  "config" jsonb NULL,
  "device_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "channels_devices_channels" FOREIGN KEY ("device_id") REFERENCES "devices" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "channel_device_id_key" to table: "channels"
CREATE UNIQUE INDEX "channel_device_id_key" ON "channels" ("device_id", "key");
-- Create "entities" table
CREATE TABLE "entities" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "name" character varying NOT NULL,
  "kind" character varying NOT NULL,
  "area_id" uuid NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "entities_areas_entities" FOREIGN KEY ("area_id") REFERENCES "areas" ("id") ON UPDATE NO ACTION ON DELETE SET NULL
);
-- Create "entity_channels" table
CREATE TABLE "entity_channels" (
  "entity_id" uuid NOT NULL,
  "channel_id" uuid NOT NULL,
  PRIMARY KEY ("entity_id", "channel_id"),
  CONSTRAINT "entity_channels_channel_id" FOREIGN KEY ("channel_id") REFERENCES "channels" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "entity_channels_entity_id" FOREIGN KEY ("entity_id") REFERENCES "entities" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
