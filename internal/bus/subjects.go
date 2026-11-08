package bus

import (
	"fmt"
	"strings"
)

// Subjects follow hub.v1.<kind>.<id>.<verb> (docs/adr/0003).
const subjectPrefix = "hub.v1."

// EntityState is where an entity's state changes are published.
func EntityState(entityID string) (string, error) { return subject("entity", entityID, "state") }

// DeviceCommand is where commands for a device are published.
func DeviceCommand(deviceID string) (string, error) { return subject("device", deviceID, "cmd") }

// IntegrationHealth is where an integration instance reports its availability.
func IntegrationHealth(integrationID string) (string, error) {
	return subject("integration", integrationID, "health")
}

func subject(kind, id, verb string) (string, error) {
	if err := validToken(id); err != nil {
		return "", fmt.Errorf("%s id: %w", kind, err)
	}
	return subjectPrefix + kind + "." + id + "." + verb, nil
}

// validToken rejects IDs that would change the subject's structure.
func validToken(s string) error {
	if s == "" {
		return fmt.Errorf("empty")
	}
	if strings.ContainsAny(s, ".*> \t\r\n") {
		return fmt.Errorf("%q contains a dot, a wildcard or whitespace", s)
	}
	return nil
}
