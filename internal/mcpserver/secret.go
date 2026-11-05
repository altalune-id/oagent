package mcpserver

import (
	"fmt"

	"github.com/google/uuid"

	"altalune.id/oagent/sealer"
)

// SECURITY: the AAD binds a sealed credential to its org and server row, so bytes copied to another row or org never open.
func authAAD(orgID, id uuid.UUID) []byte {
	return []byte("mcpserver:" + orgID.String() + ":" + id.String() + ":auth")
}

func sealAuth(sl sealer.Sealer, orgID, id uuid.UUID, value string) ([]byte, error) {
	sealed, err := sl.Seal([]byte(value), authAAD(orgID, id))
	if err != nil {
		return nil, fmt.Errorf("mcpserver.sealAuth: %w", err)
	}
	return sealed, nil
}

func openAuth(sl sealer.Sealer, orgID, id uuid.UUID, sealed []byte) (string, error) {
	plain, err := sl.Open(sealed, authAAD(orgID, id))
	if err != nil {
		return "", fmt.Errorf("mcpserver.openAuth: %w", err)
	}
	return string(plain), nil
}
