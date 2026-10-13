// Package views renders the invite email and the invite pages.
package views

import "fmt"

// InviteSubject is the subject of the invite email.
func InviteSubject(inviter, org string) string {
	return fmt.Sprintf("%s invited you to %s on oagent", inviter, org)
}

// InviteText is the plain-text invite email.
func InviteText(org, inviter, role, link, expires string) string {
	return fmt.Sprintf("%s invited you to join %s on oagent as %s.\n\n"+
		"Open this link to join:\n%s\n\n"+
		"The link works until %s. If you were not expecting this invite, you can ignore this email.\n",
		inviter, org, role, link, expires)
}
