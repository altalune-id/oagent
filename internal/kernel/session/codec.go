package session

import (
	"encoding/json"
	"fmt"

	"altalune.id/oagent/sealer"
)

// SECURITY: the payload carries identity and a future IdP token, so it is sealed and bound to its sid as additional data.
func sealPrincipal(s sealer.Sealer, sid string, p Principal) ([]byte, error) {
	plain, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("session: marshal principal: %w", err)
	}
	blob, err := s.Seal(plain, []byte(sid))
	if err != nil {
		return nil, fmt.Errorf("session: seal principal: %w", err)
	}
	return blob, nil
}

func openPrincipal(s sealer.Sealer, sid string, blob []byte) (Principal, error) {
	plain, err := s.Open(blob, []byte(sid))
	if err != nil {
		return Principal{}, fmt.Errorf("session: open principal: %w", err)
	}
	var p Principal
	if err := json.Unmarshal(plain, &p); err != nil {
		return Principal{}, fmt.Errorf("session: unmarshal principal: %w", err)
	}
	return p, nil
}
