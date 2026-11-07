package channel

import (
	"fmt"

	"github.com/google/uuid"

	"altalune.id/oagent/sealer"
)

const (
	slotPrimary   = "signing_primary"
	slotSecondary = "signing_secondary"
	slotDeviceKey = "device_key"
)

// SECURITY: the AAD binds a sealed secret to its org, connection row and slot, so bytes copied to another row, org or slot never open.
func aad(orgID, id uuid.UUID, slot string) []byte {
	return []byte("channel:" + orgID.String() + ":" + id.String() + ":" + slot)
}

func seal(sl sealer.Sealer, c *Connection, slot, value string) ([]byte, error) {
	out, err := sl.Seal([]byte(value), aad(c.OrgID, c.ID, slot))
	if err != nil {
		return nil, fmt.Errorf("channel: seal %s: %w", slot, err)
	}
	return out, nil
}

func open(sl sealer.Sealer, c *Connection, slot string, sealed []byte) (string, error) {
	plain, err := sl.Open(sealed, aad(c.OrgID, c.ID, slot))
	if err != nil {
		return "", &SecretsUnavailableError{Cause: err}
	}
	return string(plain), nil
}
