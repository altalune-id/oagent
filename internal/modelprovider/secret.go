package modelprovider

import (
	"fmt"

	"github.com/google/uuid"

	"altalune.id/oagent/sealer"
)

// SECURITY: the AAD binds a sealed key to its org and provider row, so bytes copied to another row or org never open.
func keyAAD(orgID, id uuid.UUID) []byte {
	return []byte("modelprovider:" + orgID.String() + ":" + id.String() + ":api_key")
}

func sealKey(sl sealer.Sealer, orgID, id uuid.UUID, key string) ([]byte, error) {
	sealed, err := sl.Seal([]byte(key), keyAAD(orgID, id))
	if err != nil {
		return nil, fmt.Errorf("modelprovider.sealKey: %w", err)
	}
	return sealed, nil
}

func openKey(sl sealer.Sealer, orgID, id uuid.UUID, sealed []byte) (string, error) {
	plain, err := sl.Open(sealed, keyAAD(orgID, id))
	if err != nil {
		return "", fmt.Errorf("modelprovider.openKey: %w", err)
	}
	return string(plain), nil
}
